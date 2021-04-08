## image_exits_in_repo IMAGE_URI
#
# Checks whether IMAGE_URI -- e.g. quay.io/app-sre/osd-metrics-exporter:abcd123
# -- exists in the remote repository.
# If so, returns success.
# If the image does not exist, but the query was otherwise successful, returns
# failure.
# If the query fails for any reason, prints an error and *exits* nonzero.
image_exists_in_repo() {
    local image_uri=$1
    local output

    output=$(skopeo inspect docker://${image_uri} 2>&1)
    if [[ $? -eq 0 ]]; then
        # The image exists. Sanity check the output.
        local digest=$(echo $output | jq -r .Digest)
        if [[ -z "$digest" ]]; then
            echo "Unexpected error: skopeo inspect succeeded, but output contained no .Digest"
            echo "Here's the output:"
            echo "$output"
            exit 1
        fi
        echo "Image ${image_uri} exists with digest $digest."
        return 0
    elif [[ "$output" == *"manifest unknown"* ]]; then
        # We were able to talk to the repository, but the tag doesn't exist.
        # This is the normal "green field" case.
        echo "Image ${image_uri} does not exist in the repository."
        return 1
    else
        # Any other error. For example:
        #   - "unauthorized: access to the requested resource is not
        #     authorized". This happens not just on auth errors, but if we
        #     reference a repository that doesn't exist.
        #   - "no such host".
        #   - Network or other infrastructure failures.
        # In all these cases, we want to bail, because we don't know whether
        # the image exists (and we'd likely fail to push it anyway).
        echo "Error querying the repository for ${image_uri}:"
        echo "$output"
        exit 1
    fi
}

