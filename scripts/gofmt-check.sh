#!/bin/bash

unformatted=$(gofmt -l .)
[ -z "$unformatted" ] && exit 0

echo "$unformatted"

exit 1
