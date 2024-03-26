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
8. ControlPlaneMachineSet State

# Local development without OLM

1. Create `Namespace`, `Role` and `RoleBinding`. Requires [yq](https://github.com/mikefarah/yq).

```shell
for k in "Namespace" "Role" "RoleBinding"; do;
  k=$k yq '.objects[].spec.resources[] | select(.kind==strenv(k))' \
    hack/olm-registry/olm-artifacts-template.yaml \
    | oc apply -f - ; 
done
```

2. Create `(Cluster-)Role`, `(Cluster-)RoleBinding` and `ServiceAccount`. 

```shell
oc apply -f ./deploy/10_osd-metrics-exporter.ClusterRole.yaml
oc apply -f ./deploy/10_osd-metrics-exporter_openshift-osd-metrics.Role.yaml
oc apply -f ./deploy/10_osd-metrics-exporter_openshift-osd-metrics.ServiceAccount.yaml
oc apply -f ./deploy/20_osd-metrics-exporter.ClusterRoleBinding.yaml
oc apply -f ./deploy/20_osd-metrics-exporter_openshift-osd-metrics.RoleBinding.yaml
oc apply -f ./resources/10_osd-metrics-exporter_openshift-config.Role.yaml
oc apply -f ./resources/10_osd-metrics-exporter_openshift-config.RoleBinding.yaml
```

3. Optionally authenticate as the `serviceaccount`.

```shell
# local crc cluster
oc login "$(oc get infrastructures cluster -o json | jq -r '.status.apiServerURL')" --token "$(oc create token -n openshift-osd-metrics osd-metrics-exporter)"
# openshift cluster
oc login "$(oc get infrastructures cluster -o json | jq -r '.status.apiServerURL')" --token "$(oc create token -n openshift-osd-metrics osd-metrics-exporter --as backplane-cluster-admin)"
```

4. Switch to project

```shell
oc project openshift-osd-metrics
```

5. Build and run the operator

```shell
make go-build
./build/_output/bin/osd-metrics-exporter
```

