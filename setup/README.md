# PVC for NFS storage
## 1. install helm
```
curl https://raw.githubusercontent.com/helm/helm/main/scripts/get-helm-3 | bash
```

##  2. install helm repo 
```
helm repo add nfs-subdir-external-provisioner https://kubernetes-sigs.github.io/nfs-subdir-external-provisioner/
```
## 3. create storage

Fill it with your `ServerIP` and the `NFSPATH`.
```
helm install second-nfs-subdir-external-provisioner nfs-subdir-external-provisioner/nfs-subdir-external-provisioner \
    --set nfs.server=$ServerIP \
    --set nfs.path=$NFSPATH \
    --set storageClass.name=my-nfs \
    --set storageClass.provisionerName=k8s-sigs.io/second-nfs-subdir-external-provisioner
```

## 4. create persistance storage volume
```
kubectl apply -f nfs-pvc.yaml
```

## 5. Make sure that its bound
```
kubectl get pvc
```