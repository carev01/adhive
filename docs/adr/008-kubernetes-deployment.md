# ADR-008: Kubernetes Deployment Architecture

**Status:** Proposed  
**Date:** 2026-03-10  
**Decision Makers:** @bumblebee, @jarvis  
**Related ADRs:** ADR-007 (Deployment Configuration)

---

## Executive Summary

This ADR documents the Kubernetes deployment architecture for AdHive, including Helm chart structure, security considerations, persistence strategy for SQLite, and operational concerns.

---

## Context

AdHive needs to be deployable on Kubernetes clusters. The existing Helm chart provides a foundation, but requires architectural review for:
1. SQLite persistence on PersistentVolumes
2. Horizontal scaling considerations
3. Security hardening
4. Ingress configuration for various controllers
5. Operational concerns (backup, monitoring)

---

## Decision

### 1. Helm Chart Structure

The Helm chart follows standard Kubernetes patterns with the following structure:

```
deploy/helm/adhive/
├── Chart.yaml           # Chart metadata
├── values.yaml          # Default values
├── values/
│   ├── dev.yaml        # Development overrides
│   └── prod.yaml       # Production overrides
└── templates/
    ├── _helpers.tpl    # Template helpers
    ├── deployment.yaml  # Deployment resource
    ├── service.yaml     # Service resource
    ├── ingress.yaml     # Ingress resource
    ├── configmap.yaml   # ConfigMap for env vars
    ├── secret.yaml      # Secret for SESSION_SECRET
    ├── pvc-database.yaml   # PVC for SQLite
    ├── pvc-archives.yaml   # PVC for archives
    ├── pvc-thumbnails.yaml # PVC for thumbnails
    ├── hpa.yaml         # Horizontal Pod Autoscaler
    ├── networkpolicy.yaml  # Network Policy
    └── NOTES.txt        # Post-install notes
```

### 2. Persistence Strategy

**Decision:** SQLite on PersistentVolume with ReadWriteOnce access mode.

**Rationale:**
- AdHive is designed for single-tenant/self-hosted deployments
- SQLite provides adequate performance for catalog workloads
- Simplicity of operations (no separate database to manage)
- Cost-effective (no additional database infrastructure)

**Constraints:**
- **Single replica mode** when using SQLite persistence
- HPA scaling limited to single instance for data consistency
- ReadWriteOnce PVC means no multi-zone replication

**For multi-replica deployments, the architecture must change to:**
- Use external PostgreSQL/MySQL database
- Use shared storage (ReadWriteMany) for archives/thumbnails
- Or use S3-compatible storage for archives/thumbnails

**Configuration:**

```yaml
# values.yaml
persistence:
  enabled: true
  storageClass: ""  # Use cluster default
  accessMode: ReadWriteOnce
  
  database:
    enabled: true
    size: 1Gi        # SQLite database
    mountPath: /app/data
  
  archives:
    enabled: true
    size: 10Gi       # Page archives
    mountPath: /app/data/archives
  
  thumbnails:
    enabled: true
    size: 5Gi        # Generated thumbnails
    mountPath: /app/data/thumbnails
```

### 3. Security Architecture

**Pod Security Context:**

```yaml
pod:
  securityContext:
    runAsNonRoot: true
    runAsUser: 1000
    runAsGroup: 1000
    fsGroup: 1000
    fsGroupChangePolicy: "OnRootMismatch"
```

**Container Security Context:**

```yaml
containerSecurityContext:
  allowPrivilegeEscalation: false
  readOnlyRootFilesystem: true
  capabilities:
    drop:
      - ALL
    add:
      - NET_BIND_SERVICE  # Only if binding to port 80
```

**Secrets Management:**

SESSION_SECRET is stored in a Kubernetes Secret:

```yaml
# Production: Use external secrets management
secret:
  enabled: true
  data:
    SESSION_SECRET: ""  # Set via --set or external-secrets
```

**Recommendations:**
1. Use External Secrets Operator for production
2. Integrate with HashiCorp Vault or AWS Secrets Manager
3. Never commit secrets to Git

### 4. Ingress Configuration

**Supported Ingress Controllers:**

| Controller | Annotation Support | TLS Support |
|------------|---------------------|-------------|
| nginx-ingress | Full support | cert-manager |
| Traefik | Full support | Let's Encrypt |
| HAProxy | Full support | cert-manager |
| GCE Ingress | Limited | GKE managed certs |

**Nginx Ingress Example:**

```yaml
ingress:
  enabled: true
  className: "nginx"
  annotations:
    cert-manager.io/cluster-issuer: "letsencrypt-prod"
    nginx.ingress.kubernetes.io/proxy-body-size: "50m"
    nginx.ingress.kubernetes.io/proxy-read-timeout: "300"
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

**Body Size Consideration:**
- Default `proxy-body-size` should be increased for archive uploads (50MB recommended)
- Increase `proxy-read-timeout` for large archive operations

### 5. Horizontal Pod Autoscaler

**Constraint:** HPA is incompatible with SQLite persistence in multi-replica mode.

**Single-Replica Mode (Default):**

```yaml
# values.yaml
autoscaling:
  enabled: false
  minReplicas: 1
  maxReplicas: 1
```

**Multi-Replica Mode (requires architecture changes):**

To enable HPA with multiple replicas:
1. Replace SQLite with PostgreSQL or MySQL
2. Use shared storage (ReadWriteMany) for archives/thumbnails
3. Or use S3-compatible storage

**Future Architecture for Horizontal Scaling:**

```yaml
# Future: External database + S3 storage
persistence:
  database:
    enabled: false  # Use external PostgreSQL
  archives:
    storageType: s3
    s3:
      bucket: adhive-archives
      endpoint: s3.amazonaws.com
  thumbnails:
    storageType: s3
    s3:
      bucket: adhive-thumbnails

externalDatabase:
  enabled: true
  host: postgres.example.com
  port: 5432
  database: adhive
  existingSecret: postgres-credentials
```

### 6. Resource Management

**Development Values:**

```yaml
resources:
  limits:
    cpu: 500m
    memory: 256Mi
  requests:
    cpu: 50m
    memory: 64Mi
```

**Production Values:**

```yaml
resources:
  limits:
    cpu: 2000m
    memory: 1Gi
  requests:
    cpu: 500m
    memory: 512Mi
```

**Rationale:**
- SQLite is memory-efficient
- Go binary has small footprint
- Archives loaded on-demand (not kept in memory)
- Allow burst capacity for archive processing

### 7. Health Checks

**Liveness Probe:**

```yaml
livenessProbe:
  enabled: true
  httpGet:
    path: /health
    port: http
  initialDelaySeconds: 30
  periodSeconds: 10
  timeoutSeconds: 5
  failureThreshold: 3
```

**Readiness Probe:**

```yaml
readinessProbe:
  enabled: true
  httpGet:
    path: /health
    port: http
  initialDelaySeconds: 10
  periodSeconds: 5
  timeoutSeconds: 3
  failureThreshold: 3
```

**Note:** Same endpoint (`/health`) for both probes. Consider separating:
- `/health` for liveness (is the process alive?)
- `/ready` for readiness (can it serve requests?)

### 8. Network Policy

**Default Policy:**

```yaml
networkPolicy:
  enabled: false  # Enable in production
```

**Recommended Production Policy:**

```yaml
networkPolicy:
  enabled: true
  ingress:
    - from:
        - namespaceSelector:
            matchLabels:
              name: ingress-nginx
      ports:
        - protocol: TCP
          port: 8080
  egress:
    - to:
        - namespaceSelector: {}
      ports:
        - protocol: UDP
          port: 53  # DNS
    - to:
        - ipBlock:
            cidr: 0.0.0.0/0
      ports:
        - protocol: TCP
          port: 443  # HTTPS egress for scraping
```

### 9. Backup Strategy

**PVC Backup with Velero:**

```yaml
# Velero backup schedule
apiVersion: velero.io/v1
kind: Schedule
metadata:
  name: adhive-backup
  namespace: velero
spec:
  schedule: "0 2 * * *"  # Daily at 2 AM
  template:
    includedNamespaces:
      - adhive
    excludedResources:
      - pods
    storageLocation: default
    volumeSnapshotLocations:
      - default
```

**Manual Backup Script:**

```bash
#!/bin/bash
# k8s-backup.sh

NAMESPACE="adhive"
RELEASE="adhive"

# SQLite backup
kubectl exec -n ${NAMESPACE} ${RELEASE}-0 -- \
  sqlite3 /app/data/ad-catalog.db ".backup /tmp/backup.db"

kubectl cp ${NAMESPACE}/${RELEASE}-0:/tmp/backup.db \
  ./backup-$(date +%Y%m%d).db

# PVC snapshot (if supported)
kubectl patch pvc ${RELEASE}-database \
  -n ${NAMESPACE} \
  -p '{"metadata":{"annotations":{"backup.velero.io/backup-volumes":"database"}}}'
```

### 10. Monitoring

**ServiceMonitor for Prometheus:**

```yaml
serviceMonitor:
  enabled: true
  interval: 30s
  scrapeTimeout: 10s
  namespace: monitoring
  labels:
    release: prometheus
```

**Note:** AdHive currently doesn't expose Prometheus metrics. Future enhancement needed.

**Recommended Metrics:**
- HTTP request duration
- Active sessions count
- Archive processing queue depth
- Thumbnail generation latency
- SQLite database size

---

## Implementation Checklist

### Phase 1: Core Deployment (Complete)

- [x] Chart.yaml with metadata
- [x] values.yaml with defaults
- [x] deployment.yaml template
- [x] service.yaml template
- [x] ingress.yaml template
- [x] pvc-*.yaml templates
- [x] configmap.yaml template
- [x] secret.yaml template
- [x] _helpers.tpl with template functions
- [x] dev.yaml values
- [x] prod.yaml values

### Phase 2: Security Hardening

- [x] Non-root user configuration
- [x] Read-only root filesystem
- [x] Capability dropping
- [x] Secret for SESSION_SECRET
- [ ] NetworkPolicy for production
- [ ] PodSecurityPolicy/Pod Security Standards
- [ ] ServiceAccount creation

### Phase 3: Operational Readiness

- [ ] NOTES.txt for post-install instructions
- [ ] README.md for chart documentation
- [ ] Backup with Velero integration
- [ ] Monitoring via ServiceMonitor
- [ ] Liveness/readiness probe separation

### Phase 4: Multi-Replica Support (Future)

- [ ] External database option
- [ ] S3 storage for archives/thumbnails
- [ ] Redis for session storage
- [ ] HPA configuration

---

## Security Considerations

### Current Security Posture

| Item | Status | Notes |
|------|--------|-------|
| Non-root user | ✅ | UID 1000 |
| Read-only filesystem | ✅ | EmptyDir for /tmp |
| Capability dropping | ✅ | All caps dropped |
| Secret management | ✅ | K8s Secret for SESSION_SECRET |
| Network segmentation | ⚠️ | NetworkPolicy optional |
| RBAC | ⚠️ | Uses default service account |
| Pod Security Standards | ⚠️ | Needs explicit policy |

### Recommended Security Enhancements

1. **Create dedicated ServiceAccount:**

```yaml
# templates/serviceaccount.yaml
apiVersion: v1
kind: ServiceAccount
metadata:
  name: {{ include "adhive.fullname" . }}
  labels:
    {{- include "adhive.labels" . | nindent 4 }}
automountServiceAccountToken: false
```

2. **Add PodDisruptionBudget:**

```yaml
# templates/pdb.yaml
{{- if .Values.pdb.enabled }}
apiVersion: policy/v1
kind: PodDisruptionBudget
metadata:
  name: {{ include "adhive.fullname" . }}
spec:
  minAvailable: {{ .Values.pdb.minAvailable }}
  selector:
    matchLabels:
      {{- include "adhive.selectorLabels" . | nindent 6 }}
{{- end }}
```

3. **Enable NetworkPolicy:**

```yaml
# templates/networkpolicy.yaml (enhanced)
{{- if .Values.networkPolicy.enabled }}
apiVersion: networking.k8s.io/v1
kind: NetworkPolicy
metadata:
  name: {{ include "adhive.fullname" . }}
spec:
  podSelector:
    matchLabels:
      {{- include "adhive.selectorLabels" . | nindent 6 }}
  policyTypes:
    - Ingress
    - Egress
  ingress:
    - from:
        - namespaceSelector:
            matchLabels:
              kubernetes.io/metadata.name: ingress-nginx
      ports:
        - protocol: TCP
          port: {{ .Values.service.port }}
  egress:
    # DNS resolution
    - to:
        - namespaceSelector: {}
      ports:
        - protocol: UDP
          port: 53
    # HTTPS egress for web scraping
    - to:
        - ipBlock:
            cidr: 0.0.0.0/0
      ports:
        - protocol: TCP
          port: 443
{{- end }}
```

---

## Deployment Examples

### Development Deployment

```bash
# Install with development values
helm install adhive ./deploy/helm/adhive \
  -f ./deploy/helm/adhive/values/dev.yaml \
  --set env.SESSION_SECRET=$(openssl rand -hex 32)
```

### Production Deployment

```bash
# Install with production values
helm install adhive ./deploy/helm/adhive \
  -f ./deploy/helm/adhive/values/prod.yaml \
  --set secret.data.SESSION_SECRET=$(openssl rand -hex 32) \
  --set ingress.hosts[0].host=adhive.yourdomain.com \
  --set persistence.storageClass=fast-storage
```

### Production with External Secrets

```bash
# Create secret externally first
kubectl create secret generic adhive-secret \
  --from-literal=SESSION_SECRET=$(openssl rand -hex 32)

# Install chart referencing external secret
helm install adhive ./deploy/helm/adhive \
  -f ./deploy/helm/adhive/values/prod.yaml \
  --set secret.enabled=false \
  --set externalSecret.name=adhive-secret
```

---

## Upgrade Path

### From Single Replica to Multi-Replica

This is a **breaking change** requiring data migration:

1. Export SQLite database:
   ```bash
   sqlite3 ad-catalog.db .dump > dump.sql
   ```

2. Provision PostgreSQL database

3. Import to PostgreSQL:
   ```bash
   psql adhive < <(sqlite3 ad-catalog.db .dump | \
     sed 's/INTEGER PRIMARY KEY AUTOINCREMENT/SERIAL PRIMARY KEY/g')
   ```

4. Update AdHive configuration for PostgreSQL

5. Migrate archives/thumbnails to S3 or shared storage

6. Deploy new chart with `autoscaling.enabled: true`

---

## Alternative Architectures

### Option A: SQLite on PVC (Current)

**Pros:**
- Simple deployment
- No external dependencies
- Low resource footprint

**Cons:**
- Single replica only
- No built-in replication
- Backup requires PVC snapshot

### Option B: External PostgreSQL

**Pros:**
- Multi-replica support
- Built-in replication
- Mature backup tools

**Cons:**
- Additional infrastructure
- Higher resource usage
- More complex operations

### Option C: Cloud-Native (Managed Services)

**Pros:**
- Fully managed database
- Built-in backups
- Auto-scaling

**Cons:**
- Vendor lock-in
- Higher cost
- Requires cloud expertise

---

## Appendix A: Values Schema

| Path | Type | Default | Description |
|------|------|---------|-------------|
| `image.repository` | string | `ghcr.io/adhive/adhive` | Image repository |
| `image.tag` | string | `latest` | Image tag |
| `image.pullPolicy` | string | `IfNotPresent` | Pull policy |
| `service.type` | string | `ClusterIP` | Service type |
| `service.port` | int | `8080` | Service port |
| `ingress.enabled` | bool | `true` | Enable ingress |
| `ingress.className` | string | `nginx` | Ingress class |
| `persistence.enabled` | bool | `true` | Enable PVCs |
| `persistence.storageClass` | string | `""` | Storage class |
| `persistence.database.size` | string | `1Gi` | Database PVC size |
| `persistence.archives.size` | string | `10Gi` | Archives PVC size |
| `persistence.thumbnails.size` | string | `5Gi` | Thumbnails PVC size |
| `resources.limits.cpu` | string | `1000m` | CPU limit |
| `resources.limits.memory` | string | `512Mi` | Memory limit |
| `resources.requests.cpu` | string | `100m` | CPU request |
| `resources.requests.memory` | string | `128Mi` | Memory request |
| `autoscaling.enabled` | bool | `false` | Enable HPA |
| `autoscaling.minReplicas` | int | `1` | Minimum replicas |
| `autoscaling.maxReplicas` | int | `10` | Maximum replicas |
| `secret.enabled` | bool | `true` | Enable secret creation |
| `env.SESSION_SECRET` | string | `""` | Session secret |
| `env.GO_ENV` | string | `production` | Environment |
| `env.LOG_LEVEL` | string | `info` | Log level |

---

## Appendix B: Common Issues

### Issue: Permission denied on PVC

**Cause:** Init container runs as root to fix permissions, but some storage classes don't allow this.

**Solution:** Use `fsGroupChangePolicy: OnRootMismatch` or use a storage class that supports `securityContext.fsGroup`.

### Issue: Database locked errors

**Cause:** Multiple replicas trying to write to SQLite.

**Solution:** Ensure `replicaCount: 1` and `autoscaling.enabled: false` when using SQLite.

### Issue: Ingress not working

**Cause:** Ingress class not installed or cert-manager not configured.

**Solution:** Ensure nginx-ingress and cert-manager are installed:
```bash
helm repo add ingress-nginx https://kubernetes.github.io/ingress-nginx
helm repo add jetstack https://charts.jetstack.io
helm install ingress-nginx ingress-nginx/ingress-nginx
helm install cert-manager jetstack/cert-manager --set installCRDs=true
```

---

*End of ADR-008*