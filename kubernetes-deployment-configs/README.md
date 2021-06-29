# imgdeflator Frontend

This helm chart releases imgdeflator (Frontend) to Kubernetes. Frontend services will create also an internal Load Balancer so they are reachable within our internal network and services.

## Installation and Upgrading
### Dev/Staging

Be sure that the nitro-cloud namespace exist, otherwise create it:
```bash
kubectl describe namespace nitro-cloud

# If it does not exist create with kubectl create namespace nitro-cloud
```

Install (assuming this is version 1.0):
```bash
helm install imgdeflator . -f values-dev.yaml --namespace nitro-cloud --set appVersion="1.0"
```

Upgrade (assuming this is version 1.1):
```bash
helm upgrade imgdeflator . -f values-dev.yaml --namespace nitro-cloud --set appVersion="1.1"
```
### PROD

Be sure that the nitro-cloud namespace exist, otherwise create it:
```bash
kubectl describe namespace nitro-cloud

# If it does not exist create with kubectl create namespace nitro-cloud
```

Install (assuming this is version 1.0):
```bash
helm install imgdeflator . -f values-prod.yaml --namespace nitro-cloud --set appVersion="1.0"
```

Upgrade (assuming this is version 1.1):
```bash
helm upgrade imgdeflator . -f values-prod.yaml --namespace nitro-cloud --set appVersion="1.1"
```
