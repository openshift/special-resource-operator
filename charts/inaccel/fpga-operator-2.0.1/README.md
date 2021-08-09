# InAccel FPGA Operator

Simplifying FPGA management in Kubernetes

## Installing the Chart

To install the chart with the release name `my-release`:

```console
$ helm repo add inaccel https://setup.inaccel.com/helm
$ helm install my-release inaccel/fpga-operator --set license=...
```

These commands deploy InAccel FPGA Operator on the Kubernetes cluster in the
default configuration.

> **Tip**: List all releases using `helm list`

## Uninstalling the Chart

To uninstall/delete the `my-release` deployment:

```console
$ helm uninstall my-release
```

The command removes all the Kubernetes components associated with the chart and
deletes the release.

## Parameters

The following table lists the configurable parameters of the InAccel FPGA
Operator chart and their default values.

| Parameter            | Default            |
| -------------------- | ------------------ |
| `coral.httpsProxy`   |                    |
| `coral.image`        | `inaccel/coral`    |
| `coral.logLevel`     | `info`             |
| `coral.port`         |                    |
| `coral.pullPolicy`   | `Always`           |
| `coral.resources`    |                    |
| `coral.tag`          | *APP VERSION*      |
| `daemon.image`       | `inaccel/daemon`   |
| `daemon.pullPolicy`  |                    |
| `daemon.resources`   |                    |
| `daemon.tag`         | `latest`           |
| `license`            |                    |
| `mkrt.image`         | `inaccel/mkrt`     |
| `mkrt.pullPolicy`    |                    |
| `mkrt.tag`           | `latest`           |
| `monitor.image`      | `inaccel/monitor`  |
| `monitor.port`       |                    |
| `monitor.pullPolicy` |                    |
| `monitor.resources`  |                    |
| `monitor.tag`        | `latest`           |
| `nodeSelector`       |                    |
| `root.config`        | `/etc/inaccel`     |
| `root.data`          | `/var/opt/inaccel` |
| `root.state`         | `/var/lib/inaccel` |

Specify each parameter using the `--set key=value[,key=value]` argument to
`helm install`.

Alternatively, a YAML file that specifies the values for the parameters can be
provided while installing the chart. For example,

```console
$ helm install my-release -f values.yaml inaccel/fpga-operator
```

> **Tip**: You can use the default [values.yaml](values.yaml)
