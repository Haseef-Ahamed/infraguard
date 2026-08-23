# InfraGuard Helm Chart

Deploys the complete InfraGuard drift detection and remediation platform.

## Install

```bash
helm install infraguard ./infraguard \
  --set global.vaultAddr="http://<HOST_IP>:8200"
```

## Upgrade

```bash
helm upgrade infraguard ./infraguard --set driftEngine.scanIntervalSeconds=180
```

## Uninstall

```bash
helm uninstall infraguard
```

## Configuration

See `values.yaml` for all configurable options. Key settings:

| Value | Description | Default |
|---|---|---|
| `driftEngine.enabled` | Enable drift detection DaemonSet | `true` |
| `driftEngine.scanIntervalSeconds` | Scan frequency | `300` |
| `remediation.enabled` | Enable auto-remediation engine | `true` |
| `remediation.slaMinutes` | SLA breach threshold | `30` |
| `global.vaultAddr` | Vault server address reachable from pods | required |
