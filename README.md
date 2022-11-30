# Openshift Dedicated Metrics Exporter

A prometheus exporter to expose metrics about various features used in Openshift Dedicated Clusters.

## Current Metrics

1. Identity Provider
2. Cluster Admin
3. Limited Support
4. Cluster Proxy
5. Cluster Proxy CA Expiry Timestamp
6. Cluster Proxy CA Valid
7. Cluster ID

# Local development without OLM

1. Create `Namespace`, `Role` and `RoleBinding`. Requires [yq](https://github.com/mikefarah/yq).

```
$ for k in "Namespace" "Role" "RoleBinding"; do; k=$k yq '.objects[].spec.resources[] | select(.kind==strenv(k))' < hack/olm-registry/olm-artifacts-template.yaml|  oc create -f -; done
```

2. Create `ClusterRole`, `ClusterRoleBinding` and `ServiceAccount`. 

```
$ oc -n openshift-osd-metrics create -f deploy/cluster_role_binding.yaml
clusterrolebinding.rbac.authorization.k8s.io/osd-metrics-exporter created
$ oc -n openshift-osd-metrics create -f deploy/cluster_role.yaml
clusterrole.rbac.authorization.k8s.io/osd-metrics-exporter created
$ oc -n openshift-osd-metrics create -f deploy/service_account.yaml
serviceaccount/osd-metrics-exporter created
```

3. Requires operator-sdk >= 1.X

```
$ operator-sdk version
operator-sdk version: "v1.25.2", commit: "b63b921837de8dd6ce480033e427ecfc5e34abcc", kubernetes version: "v1.25.0", go version: "go1.19.3", GOOS: "darwin", GOARCH: "arm64"
```

4. Optionally authenticate as the `serviceaccount`.

```
$ oc login "$(oc get infrastructures cluster -o json | jq -r '.status.apiServerURL')" --token "$(oc create token -n openshift-osd-metrics osd-metrics-exporter --as backplane-cluster-admin)"
```

5. Switch to project

```
$ oc project openshift-osd-metrics
Now using project "openshift-osd-metrics" on server "https://api.sno.dofinn.xyz:6443".
```

6. Run the operator

```
$ operator-sdk run --local
INFO[0000] Running the operator locally in namespace openshift-osd-metrics.
{"level":"info","ts":1655546577.210883,"logger":"cmd","msg":"Operator Version: 0.0.1"}
{"level":"info","ts":1655546577.2109017,"logger":"cmd","msg":"Go Version: go1.18.2"}
{"level":"info","ts":1655546577.2109056,"logger":"cmd","msg":"Go OS/Arch: linux/amd64"}
...
```