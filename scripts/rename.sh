#!/bin/bash

set -e 

# shellcheck disable=SC2045
for i  in $(ls [0-9][0-9][0-9][0-9].yaml)
do
  K=$(grep ^kind "$i")
  N=$(grep -e '^  name: ' "$i" | head -n1)
  F=$(echo "${i%.*}"_"${K##* }"_"${N##* }".yaml | tr '[:upper:]' '[:lower:]')
  mv "$i" "$F"
done
