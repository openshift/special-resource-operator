#!/bin/bash

set -e

apt update -y
apt install shellcheck

make lint
