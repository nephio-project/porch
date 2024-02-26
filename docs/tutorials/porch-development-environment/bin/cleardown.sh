#! /bin/bash

kind delete cluster --name management
kind delete cluster --name edge1

rm ~/.kube/kind-management-config
rm ~/.kube/kind-edge1-config