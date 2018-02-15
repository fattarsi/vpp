#!/usr/bin/env bash

sed -i "s@contivvpp/cri@prod-contiv-cri:specific@g" ./k8s/cri-install.sh
