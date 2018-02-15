#!/bin/bash
rm -rf $HOME/.kube
sudo kubeadm reset
sudo -E kubeadm init --token-ttl 0
mkdir -p $HOME/.kube
sudo cp -i /etc/kubernetes/admin.conf $HOME/.kube/config
sudo chown $(id -u):$(id -g) $HOME/.kube/config
# TODO: wait for tiller to come up and install the chart
# Requires helm 2.8.1+
# helm init --net-host --wait
# helm install --name contiv-vpp k8s/contiv-vpp
kubectl apply -f https://raw.githubusercontent.com/contiv/vpp/master/k8s/contiv-vpp.yaml
kubectl get pods --all-namespaces
kubectl taint nodes --all node-role.kubernetes.io/master-


