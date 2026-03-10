# AdHive Helm Chart

A Helm chart for deploying AdHive on Kubernetes.

## Prerequisites

- Kubernetes 1.21+
- Helm 3.0+
- PV provisioner support (if using persistence)
- Ingress controller (nginx-ingress recommended)
- cert-manager (for TLS)

## Quick Start

### Development Deployment

```bash
# Clone the repository
git clone https://github.com/adhive/adhive.git
cd adhive

# Install with development values
helm install adhive ./deploy/helm/adhive \
  -f ./deploy/helm/adhive/values/dev.yaml \
  --set env.SESSION_SECRET=$(openssl rand -hex 32) \
  --namespace adhive --create-namespace
```

### Production Deployment

```bash
# Install with production values
helm install adhive ./deploy/helm/adhive \
  -f ./deploy/helm/adhive/values/prod.yaml \
  --set secret.data.SESSION_SECRET=$(openssl rand -hex 32) \
  --set ingress.hosts[0].host=adhive.yourdomain.com \
  --set persistence.storageClass=fast-storage \
  --namespace adhive --create-namespace
```

## Configuration

### Required Settings

| Setting | Description | How to Set |
|---------|-------------|-------------|
| `secret.data.SESSION_SECRET` | Session signing secret (32+ hex chars) | `--set` or external secret |
| `ingress.hosts[0].host` | Your domain name | `--set` or values override |
| `persistence.storageClass` | Your cluster's storage class | `--set` or values override |

### Values Files

| File | Purpose |
|------|---------|
| `values.yaml` | Default values |
| `values/dev.yaml` | Development overrides |
| `values/prod.yaml` | Production overrides |

### Key Configuration Options

```yaml
# Image configuration
image:
  repository: ghcr.io/adhive/adhive
  tag: latest
  pullPolicy: IfNotPresent

# Service configuration
service:
  type: ClusterIP
  port: 8080

# Ingress configuration
ingress:
  enabled: true
  className: nginx
  hosts:
    - host: adhive.yourdomain.com
      paths:
        - path: /
          pathType: Prefix

# Persistence (SQLite on PVC)
persistence:
  enabled: true
  storageClass: ""  # Use cluster default
  database:
    size: 1Gi
  archives:
    size: 10Gi
  thumbnails:
    size: 5Gi

# Resources
resources:
  limits:
    cpu: 1000m
    memory: 512Mi
  requests:
    cpu: 100m
    memory: 128Mi

# Security
pod:
  securityContext:
    runAsNonRoot: true
    runAsUser: 1000
    runAsGroup: 1000
    fsGroup: 1000

containerSecurityContext:
  allowPrivilegeEscalation: false
  readOnlyRootFilesystem: true
  capabilities:
    drop:
      - ALL
```

## Persistence

AdHive uses SQLite for its database, stored on a PersistentVolume.

### Single Replica Mode (Default)

When using SQLite on PVC:
- **Replicas:** 1 (must be single instance)
- **Access Mode:** ReadWriteOnce
- **Storage Class:** Any that supports RWO

### Multi-Replica Mode (Future)

To run multiple replicas:
1. Replace SQLite with PostgreSQL
2. Use shared storage (ReadWriteMany) or S3 for archives/thumbnails
3. Enable HPA

## Security

### Session Secret

The `SESSION_SECRET` must be set for secure authentication:

```bash
# Generate a secure secret
openssl rand -hex 32

# Set via --set
helm install adhive ./deploy/helm/adhive \
  --set secret.data.SESSION_SECRET=<your-secret>

# Or use external secret
kubectl create secret generic adhive-secret \
  --from-literal=SESSION_SECRET=<your-secret>

helm install adhive ./deploy/helm/adhive \
  --set secret.enabled=false \
  --set externalSecret.name=adhive-secret
```

### Network Policy

Enable network policy for production:

```yaml
networkPolicy:
  enabled: true
```

## Ingress

### Nginx Ingress (Recommended)

```yaml
ingress:
  enabled: true
  className: nginx
  annotations:
    cert-manager.io/cluster-issuer: letsencrypt-prod
    nginx.ingress.kubernetes.io/proxy-body-size: 50m
  hosts:
    - host: adhive.yourdomain.com
      paths:
        - path: /
          pathType: Prefix
  tls:
    - secretName: adhive-tls
      hosts:
        - adhive.yourdomain.com
```

### Traefik Ingress

```yaml
ingress:
  enabled: true
  className: traefik
  annotations:
    traefik.ingress.kubernetes.io/router.entrypoints: websecure
    traefik.ingress.kubernetes.io/router.tls: "true"
  hosts:
    - host: adhive.yourdomain.com
      paths:
        - path: /
          pathType: Prefix
```

## Upgrading

```bash
# Upgrade with new values
helm upgrade adhive ./deploy/helm/adhive \
  -f ./deploy/helm/adhive/values/prod.yaml \
  --set image.tag=v1.2.0

# Upgrade with new chart version
helm repo update
helm upgrade adhive adhive/adhive \
  --version 0.2.0
```

## Uninstallation

```bash
# Uninstall the release
helm uninstall adhive --namespace adhive

# Note: PVCs are not deleted by default
# To delete PVCs:
kubectl delete pvc -l app.kubernetes.io/instance=adhive -n adhive
```

## Backup

### Using Velero

```bash
# Install Velero
velero install --provider aws --bucket your-bucket --secret-file ./credentials-velero

# Create backup schedule
velero schedule create adhive-backup --schedule="0 2 * * *" --include-namespaces adhive
```

### Manual Backup

```bash
# Backup SQLite database
kubectl exec -n adhive adhive-0 -- \
  sqlite3 /app/data/ad-catalog.db ".backup /tmp/backup.db"

kubectl cp adhive/adhive-0:/tmp/backup.db ./backup-$(date +%Y%m%d).db
```

## Troubleshooting

### Pod not starting

```bash
# Check pod status
kubectl get pods -n adhive

# Check pod logs
kubectl logs -n adhive adhive-0

# Check events
kubectl describe pod -n adhive adhive-0
```

### Database permission errors

```bash
# Check init container logs
kubectl logs -n adhive adhive-0 -c init-permissions

# Verify PVC is mounted
kubectl exec -n adhive adhive-0 -- ls -la /app/data
```

### Ingress not working

```bash
# Check ingress status
kubectl get ingress -n adhive

# Check ingress controller logs
kubectl logs -n ingress-nginx -l app.kubernetes.io/name=ingress-nginx
```

## Contributing

1. Fork the repository
2. Make your changes
3. Run helm lint: `helm lint deploy/helm/adhive`
4. Submit a pull request

## License

MIT License - see LICENSE file for details.