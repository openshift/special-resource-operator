#!/bin/bash 

set -e 

for i  in $(ls [0-9][0-9][0-9][0-9].yaml)
do
  KIND=$(yq .kind $i | tr -d '"');
  NAME=$(yq .metadata.name $i | tr -d '"');
  F=$(echo ${i%.*}_${KIND}_${NAME}.yaml | tr '[:upper:]' '[:lower:]'); 
  mv $i $F
done
