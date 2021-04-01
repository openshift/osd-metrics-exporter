#!/bin/bash

set -exv

BRANCH_CHANNEL="$1"
QUAY_IMAGE="$2"

GIT_HASH=$(git rev-parse --short=7 HEAD)
GIT_COMMIT_COUNT=$(git rev-list $(git rev-list --max-parents=0 HEAD)..HEAD --count)

# Get the repo URI + image digest
REPO_DIGEST=$(docker image inspect ${QUAY_IMAGE}:${GIT_HASH} --format '{{index .RepoDigests 0}}')
if [[ -z "$REPO_DIGEST" ]]; then
    echo "Couldn't discover REPO_DIGEST for ${QUAY_IMAGE}:${GIT_HASH}!"
    exit 1
fi

# clone bundle repo
SAAS_OPERATOR_DIR="saas-osd-metrics-exporter"
BUNDLE_DIR="$SAAS_OPERATOR_DIR/osd-metrics-exporter/"

rm -rf "$SAAS_OPERATOR_DIR"

git clone \
    --branch "$BRANCH_CHANNEL" \
    https://app:"${APP_SRE_BOT_PUSH_TOKEN}"@gitlab.cee.redhat.com/service/saas-osd-metrics-exporter-bundle.git \
    "$SAAS_OPERATOR_DIR"

# ensure bundle directory exists when repository is new
mkdir -p "$BUNDLE_DIR"

# remove any versions more recent than deployed hash
REMOVED_VERSIONS=""
if [[ "$REMOVE_UNDEPLOYED" == true ]]; then
    DEPLOYED_HASH=$(
        curl -s "https://gitlab.cee.redhat.com/service/app-interface/raw/master/data/services/osd-operators/cicd/saas/saas-osd-metrics-exporter.yaml" | \
	    docker run --rm -i quay.io/app-sre/yq yq r - 'resourceTemplates[*].targets(namespace.$ref==/services/osd-operators/namespaces/hivep01ue1/cluster-scope.yml).ref'
    )

    delete=false
    # Sort based on commit number
    for version in $(ls $BUNDLE_DIR | sort -t . -k 3 -g); do
        # skip if not directory
        [ -d "$BUNDLE_DIR/$version" ] || continue

        if [[ "$delete" == false ]]; then
            short_hash=$(echo "$version" | cut -d- -f2)

            if [[ "$DEPLOYED_HASH" == "${short_hash}"* ]]; then
                delete=true
            fi
        else
            rm -rf "${BUNDLE_DIR:?BUNDLE_DIR var not set}/$version"
            REMOVED_VERSIONS="$version $REMOVED_VERSIONS"
        fi
    done
fi

# generate bundle
PREV_VERSION=$(ls "$BUNDLE_DIR" | sort -t . -k 3 -g | tail -n 1)
PREV_OPERATOR_VERSION="osd-metrics-exporter.v${PREV_VERSION}"

./hack/generate-operator-bundle.py \
    "$BUNDLE_DIR" \
    "$PREV_VERSION" \
    "$GIT_COMMIT_COUNT" \
    "$GIT_HASH" \
    "$REPO_DIGEST"

NEW_VERSION=$(ls "$BUNDLE_DIR" | sort -t . -k 3 -g | tail -n 1)
NEW_OPERATOR_VERSION="osd-metrics-exporter.v${NEW_VERSION}"

if [ "$NEW_VERSION" = "$PREV_VERSION" ]; then
    # stopping script as that version was already built, so no need to rebuild it
    exit 0
fi

# create package yaml
cat <<EOF > $BUNDLE_DIR/osd-metrics-exporter.package.yaml
packageName: osd-metrics-exporter
channels:
- name: $BRANCH_CHANNEL
  currentCSV: $NEW_OPERATOR_VERSION 
EOF

# add, commit & push
pushd $SAAS_OPERATOR_DIR

git add .

MESSAGE="add version $GIT_COMMIT_COUNT-$GIT_HASH

replaces $PREV_VERSION
removed versions: $REMOVED_VERSIONS"

git commit -m "$MESSAGE"
git push origin "$BRANCH_CHANNEL"

popd

# build the registry image
REGISTRY_IMG="quay.io/app-sre/osd-metrics-exporter-registry"
DOCKERFILE_REGISTRY="Dockerfile.olm-registry"

cat <<EOF > $DOCKERFILE_REGISTRY
FROM quay.io/openshift/origin-operator-registry:4.5

COPY $SAAS_OPERATOR_DIR manifests
RUN initializer --permissive

CMD ["registry-server", "-t", "/tmp/terminate.log"]
EOF

CATALOG_IMAGE="${REGISTRY_IMG}:${BRANCH_CHANNEL}-latest"
docker build -f $DOCKERFILE_REGISTRY --tag "$CATALOG_IMAGE" .

cleanup() {
   docker rm -f catalog-image || true
}

trap cleanup EXIT
cleanup
docker run --name catalog-image -d --rm --network host "$CATALOG_IMAGE"
sleep 10

REPLACES_VERSION=$(docker run --rm --network=host quay.io/rogbas/grpcurl -plaintext localhost:50051 api.Registry/ListBundles | jq -r 'select(.csvName == "'"$NEW_OPERATOR_VERSION"'" ) | .replaces')

if [[ "$REPLACES_VERSION" != "$PREV_OPERATOR_VERSION" ]]; then
    echo "replaces field '$REPLACES_VERSION' in catalog does not match previous version $PREV_OPERATOR_VERSION"
    exit 1
fi

# push image
skopeo copy --dest-creds "${QUAY_USER}:${QUAY_TOKEN}" \
    "docker-daemon:${REGISTRY_IMG}:${BRANCH_CHANNEL}-latest" \
    "docker://${REGISTRY_IMG}:${BRANCH_CHANNEL}-latest"

skopeo copy --dest-creds "${QUAY_USER}:${QUAY_TOKEN}" \
    "docker-daemon:${REGISTRY_IMG}:${BRANCH_CHANNEL}-latest" \
    "docker://${REGISTRY_IMG}:${BRANCH_CHANNEL}-${GIT_HASH}"
