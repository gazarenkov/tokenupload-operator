## tokenupload-operator
This is the Kubernetes controller which works with Red Hat AppStudio SPI Operator.
The controller makes it possible to upload Access Token (such as GitHub PAC) using K8s API.

**NOTE: Token Upload controller is totally adopted to SPI Operator. It is not intended to be used as a standalone operator
in any kind of production. At best (if SPI team approves) the controller will be added to the SPI Operator**
 
### Description

This controller updates/adds the Access Token to the secret storage (Vault) with the value sent with labelled K8s Secret and either:

- Update SPIAccessToken if there is a spiTokenName field in the Secret or
- Creates ready to use SPIAccessToken for the certain instance of Service Provider thanks to providerUrl field
 
### Running on the cluster
You’ll need a Kubernetes cluster to run against. The only environment it was tested against 
(for the time being) is Minikube, running as the process outside of the cluster. 
Red Hat AppStudio SPI-operator should be installed here as a prerequisite.
Operator uses Vault backend to store secrets and connects to the remote Vault instance using with following environment variables:
  
 **VAULTHOST** : URL to the Vault server, to get it you can call ./hack/vault-host.sh
  In case if it is not reachable on operator start time you will get the error like:
 
 **VAULTINSECURETLS** : whether the Vault tls connection can use untrusted certificate [false]
  ERROR   setup   failed to log in to the token storage   {"error": "wrapped storage error: error while authenticating: 
  unable to log in to auth method: unable to log in with app role auth: Put \"https://vault.192.168.64.6.nip.io/v1/auth/approle/login\": 
  x509: “Kubernetes Ingress Controller Fake Certificate” certificate is not trusted"}
 **VAULTAPPROLEROLEIDFILEPATH** : path to the file storing Vault approle's ROLE_ID for authentication [/etc/spi/role_id]
  It has to be created upfront and filled with appropriate approle's role_id (same as stored in the SPI Operator's container)
 **VAULTAPPROLESECRETIDFILEPATH** : path to the file storing Vault approle's SECRET_ID for authentication [/etc/spi/secret_id]
  It has to be created upfront and filled with appropriate approle's secret_id (same as stored in the SPI Operator's container)

To run the Operator using current context of your kubeconfig outside of the cluster:
```sh
make run VAULTHOST=https://vault.192.168.64.6.nip.io VAULTAPPROLEROLEIDFILEPATH=.secretdata/role_id VAULTAPPROLESECRETIDFILEPATH=.secretdata/secret_id VAULTINSECURETLS=true
```


### Test It Out

No additional Custom Resources required, controller is managing K8s Secrets labeled with:
spi.appstudio.redhat.com/token: ""

For example:
```sh
cat <<EOF | kubectl apply -f - 
apiVersion: v1
kind: Secret
metadata:
  name: secret-name
  labels:
    spi.appstudio.redhat.com/token: ""
type: Opaque
stringData:
  providerUrl: https://github.com
  #spiTokenName: <the name of existed token instead of provider url>
  tokenData: <token data>

EOF
```

What's important:

- the secret is labeled with **spi.appstudio.redhat.com/token: ""**  
- secret data should contain **tokenData** field  with actual token from the Service Provider to store. 
For example, to reach a private GitHub repository, you should get the Personal Access Token from your GitHub/Settings/Developer Settings/Personal access tokens.
- secret data should contain either **providerUrl** with Service Provider base URL or **spiTokenName** with the name of SPIAccessToken object to update

After creating the Secret, controller reads the token, either updates or creates new secret in the Vault database and 
updates or creates SPIAccessToken with the reference to the Vault record and immediately deletes the Secret
independently on whether the operation is succesful or not.
In a case if error occurs and update not happened new Event with the diagnostic information created 
(which will be deleted by Kubernetes later). 

## Errors


While creating SPIAccessToken - if token is invalid (on GitHub):
 Error Message:  failed to persist github metadata: metadata cache error: fetching token data: failed to list github repositories: GET https://api.github.com/user/repos?per_page=100: 401 Bad credentials []    │
│Error Reason:   MetadataFailure 

## License

Copyright 2022.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.

