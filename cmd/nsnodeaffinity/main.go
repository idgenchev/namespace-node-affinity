package main

import (
	"fmt"
	"io/ioutil"
	"net/http"
	"time"

	"github.com/idgenchev/namespace-node-affinity/pkg/affinityinjector"
	"github.com/jessevdk/go-flags"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	log "github.com/sirupsen/logrus"
	k8sclient "k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
)

var opts struct {
	Port          int           `long:"port" short:"p" env:"PORT" default:"8443" description:"The port on which to serve."`
	ReadTimeout   time.Duration `long:"read-timeout" default:"10s" description:"Read timeout"`
	WriteTimeout  time.Duration `long:"write-timeout" default:"10s" description:"Write timeout"`
	CertFile      string        `lond:"cert" short:"c" env:"CERT" default:"/etc/webhook/certs/tls.crt" description:"Path to the cert file"`
	KeyFile       string        `lond:"key" short:"k" env:"KEY" default:"/etc/webhook/certs/tls.key" description:"Path to the key file"`
	ConfigMapName string        `long:"config-map-name" short:"m" env:"CONFIG_MAP_NAME" default:"namespace-node-affinity" description:"Name of the configm map containing the node selector terms to be applied to every pod on creation. This config map should be present in every namespace where this webhook is enabled"`
}

type injectorInterface interface {
	Mutate(body []byte) ([]byte, error)
}

type handler struct {
	injector injectorInterface
}

func (h *handler) mutate(w http.ResponseWriter, r *http.Request) {
	body, err := ioutil.ReadAll(r.Body)
	defer r.Body.Close()

	if err != nil {
		log.Errorf("error reading request: %s", err)
		w.WriteHeader(http.StatusInternalServerError)
		fmt.Fprintf(w, "%s", err)
	}

	mutated, err := h.injector.Mutate(body)
	if err != nil {
		log.Error(err)
		w.WriteHeader(http.StatusInternalServerError)
		fmt.Fprintf(w, "%s", err)
	}

	w.WriteHeader(http.StatusOK)
	w.Write(mutated)
}

func main() {
	flags.Parse(&opts)

	mux := http.NewServeMux()

	config, err := rest.InClusterConfig()
	if err != nil {
		log.Fatalf("Failed to create k8s config: %s", err)
	}

	clientset, err := k8sclient.NewForConfig(config)
	if err != nil {
		log.Fatalf("Failed to create k8s client: %s", err)
	}

	h := handler{
		affinityinjector.NewAffinityInjector(clientset, opts.ConfigMapName),
	}
	mux.HandleFunc("/mutate", h.mutate)

	mux.Handle("/metrics", promhttp.Handler())

	s := &http.Server{
		Addr:           fmt.Sprintf(":%d", opts.Port),
		Handler:        mux,
		ReadTimeout:    opts.ReadTimeout,
		WriteTimeout:   opts.WriteTimeout,
		MaxHeaderBytes: 1 << 20, // 1048576; 1MiB
	}

	log.Fatal(s.ListenAndServeTLS(opts.CertFile, opts.KeyFile))
}
