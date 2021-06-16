# ping-pong

This SpecialResource recipe is made as an example of how cert-manager can be used to create and TLS certificates and keys for encrypted TLS between SpecialResource components. An example use-case is for encrypted gRPC calls between parts of a CSI driver for a software defined storage solution that requires a kernel module.

The SpecialResource is a simple client and server which ping eachother using mTLS encrypted gRPC calls. The container images are created from the Dockerfiles in https://github.com/dagrayvid/pingpong, and the code is inspired by https://github.com/ArangoGutierrez/pingpong and https://dev.to/techschoolguru/how-to-secure-grpc-connection-with-ssl-tls-in-go-4ph. 

Before it can be used, the manifest/0001_secret.yaml needs to be populated with an RSA key and a certificate, which will be used as the root certificate by the Issuer. The issuer [could also be self signed](https://cert-manager.io/docs/configuration/selfsigned/).

The following commands can be used to populate the secret:
```
openssl genrsa -out ca.key 2048
openssl req -x509 -new -nodes -key ca.key -subj "/CN=pingpong-ca" -days 10000 -out ca.crt
sed s"/tls.key:.*/tls.key: $$(cat ca.key|base64 -w 0)/" -i manifests/0001_secret.yaml
sed s"/tls.crt:.*/tls.crt: $$(cat ca.crt|base64 -w 0)/" -i manifests/0001_secret.yaml
```
