package main

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

var (
	readerErr = "reader error"
	mutateErr = "mutate error"
)

type FakeInjector struct {
	body []byte
	err  error
}

func (f *FakeInjector) Mutate(body []byte) ([]byte, error) {
	return f.body, f.err
}

type errReader struct {
}

func (r errReader) Read(p []byte) (n int, err error) {
	return 0, errors.New(readerErr)
}

func TestMutateWithRequestError(t *testing.T) {
	t.Parallel()

	h := handler{
		injector: &FakeInjector{},
	}

	req := httptest.NewRequest(http.MethodPost, "/mutate", errReader{})
	rec := httptest.NewRecorder()

	h.mutate(rec, req)

	assert.Equal(t, http.StatusInternalServerError, rec.Code)
	assert.Equal(t, readerErr, rec.Body.String())
}

func TestMutateWithMutatorError(t *testing.T) {
	t.Parallel()

	h := handler{
		injector: &FakeInjector{
			err: errors.New(mutateErr),
		},
	}

	rdr := strings.NewReader("testing")
	req := httptest.NewRequest(http.MethodPost, "/mutate", rdr)
	rec := httptest.NewRecorder()

	h.mutate(rec, req)

	assert.Equal(t, http.StatusInternalServerError, rec.Code)
	assert.Equal(t, mutateErr, rec.Body.String())
}

func TestMutate(t *testing.T) {
	t.Parallel()

	h := handler{
		injector: &FakeInjector{
			body: []byte("test"),
		},
	}

	rdr := strings.NewReader("testing")
	req := httptest.NewRequest(http.MethodPost, "/mutate", rdr)
	rec := httptest.NewRecorder()

	h.mutate(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)
	assert.Equal(t, "test", rec.Body.String())
}
