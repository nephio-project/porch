---
title: "Cert Manager Webhooks"
type: docs
weight: 2
description: "Configure cert-manager for automatic TLS certificate management in Porch webhooks"
---

Porch includes validating webhooks that require TLS certificates. By default, Porch generates self-signed certificates, but you can configure it to use cert-manager for automatic certificate management.

## Porch Webhooks

Porch uses two validating webhooks:

1. **PackageRevision Deletion Webhook** - Validates PackageRevision deletion requests
2. **Repository Validation Webhook** - Validates Repository creation and updates

## Using the Catalog Package

The Nephio catalog provides a ready-to-use package for Porch with cert-manager integration:

```bash
# Register the catalog repository (if not already done)
porchctl repo register \
  --namespace default \
  https://github.com/nephio-project/catalog.git \
  --name=catalog

# Clone the cert-manager webhook package
porchctl rpkg clone \
  catalog.nephio.optional.porch-cert-manager-webhook.main \
  porch-cert-manager \
  --repository=deployment-repo \
  --namespace=default
```

This package includes:
- Self-signed Certificate Issuer
- Certificate resource for webhook TLS
- Porch Server deployment with cert-manager integration
- All necessary RBAC and service configurations

## Manual Configuration

Alternatively, you can configure cert-manager integration manually:

### Default: Self-Signed Certificates

By default, Porch automatically generates self-signed TLS certificates for its webhooks. No additional configuration is required.

### Cert-Manager Integration

#### Prerequisites

- [cert-manager](https://cert-manager.io/docs/installation/) installed in your cluster
- A configured Issuer or ClusterIssuer

#### Configuration

1. **Enable cert-manager webhook support** by setting the environment variable:

```yaml
env:
- name: USE_CERT_MAN_FOR_WEBHOOK
  value: "true"
```

2. **Create a Certificate resource** for the webhook (based on the catalog package):

```yaml
apiVersion: cert-manager.io/v1
kind: Issuer
metadata:
  name: my-ca-issuer
  namespace: porch-system
spec:
  selfSigned: {}
---
apiVersion: cert-manager.io/v1
kind: Certificate
metadata:
  name: porch-system-server-certificate
  namespace: porch-system
spec:
  isCA: true
  commonName: my-selfsigned-ca
  secretName: porch-system-server-tls
  duration: 8760h # 365 days
  renewBefore: 8640h # 360 days
  issuerRef:
    name: my-ca-issuer
    kind: Issuer
    group: cert-manager.io
  dnsNames:
  - api.porch-system.svc
  - api.porch-system.svc.cluster.local
```

3. **Mount the certificate secret** in the Porch server deployment:

```yaml
spec:
  template:
    spec:
      containers:
      - name: porch-server
        env:
        - name: USE_CERT_MAN_FOR_WEBHOOK
          value: "true"
        - name: CERT_STORAGE_DIR
          value: "/etc/webhook/certs"
        volumeMounts:
        - name: webhook-certs
          mountPath: /etc/webhook/certs
          readOnly: true
      volumes:
      - name: webhook-certs
        secret:
          secretName: porch-system-server-tls
```

## Environment Variables

Porch webhook configuration supports these environment variables:

| Variable | Default | Description |
|----------|---------|-------------|
| `USE_CERT_MAN_FOR_WEBHOOK` | `false` | Enable cert-manager certificate management |
| `CERT_STORAGE_DIR` | `/tmp/cert` | Directory where certificates are stored |

{{% alert title="Note" color="primary" %}}
The catalog package uses `/etc/webhook/certs` as the certificate directory, while the default is `/tmp/cert`. Make sure your `CERT_STORAGE_DIR` matches your volume mount path.
{{% /alert %}}

| `WEBHOOK_SERVICE_NAME` | Auto-detected | Kubernetes service name for webhooks |
| `WEBHOOK_SERVICE_NAMESPACE` | Auto-detected | Kubernetes service namespace |
| `WEBHOOK_PORT` | `8443` | Port for webhook server |
| `WEBHOOK_HOST` | `localhost` | Webhook host (for URL-based webhooks) |

## Certificate Rotation

When using cert-manager:
- Certificates are automatically renewed before expiration
- Porch watches the certificate directory and reloads certificates when they change
- No manual intervention required for certificate rotation

## Troubleshooting

### Check Webhook Status

```bash
# Check ValidatingWebhookConfiguration
kubectl get validatingwebhookconfiguration

# Check webhook endpoints
kubectl get validatingwebhookconfiguration packagerev-deletion-validating-webhook -o yaml
kubectl get validatingwebhookconfiguration repository-validating-webhook -o yaml
```

### Check Certificate Status

```bash
# Check Certificate resource
kubectl get certificate -n porch-system

# Check certificate details
kubectl describe certificate porch-webhook-cert -n porch-system

# Check secret contents
kubectl get secret porch-system-server-tls -n porch-system -o yaml
```

### Common Issues

- **Webhook timeouts**: Ensure the webhook service is running and accessible
- **Certificate errors**: Verify cert-manager is properly configured and the Certificate resource is ready
- **DNS issues**: Ensure the certificate includes all necessary DNS names for the webhook service