#!/usr/bin/env bash

set -euo pipefail

# Usage {{{
usage() {
    cat <<EOF
Usage: $(basename "$0") [OPTIONS]
  Options:
        -P <project_id>      Gitlab project ID
        -g <gitlab_url>      URL of the Gitlab instance, e.g. https://gitlab.example.com
        -n <artifact_name>   Name of the Gitlab artifact, should be a zip file
        -t <gitlab_token>    Gitlab API token - Preferably use the environment variable GITLAP_API_TOKEN
        -j <job_name>        Gitlab job name, which contains the composer archive
        -a <package_api_url> Password for the packagist API - Preferably use the environment variable ADMIN_AUTH_PASSWORD
        -u <admin_user>      User for the packagist API - Prefarbly use the environment variable ADMIN_AUTH_USERNAME
        -p <admin_password>  Password for the packagist API - Preferably use the environment variable ADMIN_AUTH_PASSWORD
        -h                   Display this help text
  Any unrecognized option will lead to an error.
EOF
}
# }}}

# Environment variables {{{
gitlab_token=${GITLAP_API_TOKEN:-}
admin_user=${ADMIN_AUTH_USERNAME:-}
admin_password=${ADMIN_AUTH_PASSWORD:-}
# }}}

# Cli arguments {{{
while [[ $# -gt 0 ]]; do
    case $1 in
    -P)
        project_id="$2"
        shift;shift;;
    -g)
        gitlab_url="$2"
        shift;shift;;
    -n)
        artifact_name="$2"
        shift;shift;;
    -t)
        gitlab_token="$2"
        shift;shift;;
    -j)
        job_name="$2"
        shift;shift;;
    -u)
        admin_user="$2"
        shift;shift;;
    -p)
        admin_password="$2"
        shift;shift;;
    -a)
        package_api_url="$2"
        shift;shift;;
    -h)
        usage
        exit 1;;
    -*)
        echo "Unknown option $1"
        echo
        usage
        exit 1;;
    esac
done
# }}}

# Argument validation {{{
for required_argument in "project_id" "gitlab_url" "artifact_name" "gitlab_token" "job_name" "admin_user" "admin_password" "package_api_url"; do
    if [[ "${!required_argument:-}" == "" ]]; then
        echo "Missing value for ${required_argument}"
        echo
        usage
        exit 1
    fi
done
# }}}

# Temporary local artifact folder {{{
download_dir=$(mktemp -d)
# }}}

# Copy Gitlab artifacts to packagist {{{
readarray -t project_tags < <(curl -sf --location --header "PRIVATE-TOKEN: ${gitlab_token}" "${gitlab_url}/api/v4/projects/${project_id}/repository/tags" | jq -r "map(.name) | .[]")

if [[ "${#project_tags[@]}" == "0" ]]; then
    echo "Did not find any tags for the given project ${project_id}"
    exit 1
fi

for tag in "${project_tags[@]}"; do
    artifact_path="${download_dir}/${tag}-${artifact_name}"
    echo "Downloading ${artifact_name} for tag ${tag} to ${artifact_path}"
    echo curl -s --location --output "${artifact_path}" --header "PRIVATE-TOKEN: ${gitlab_token}" "${gitlab_url}/api/v4/projects/${project_id}/jobs/artifacts/${tag}/raw/${artifact_name}?job=${job_name}"
    curl -s --location --output "${artifact_path}" --header "PRIVATE-TOKEN: ${gitlab_token}" "${gitlab_url}/api/v4/projects/${project_id}/jobs/artifacts/${tag}/raw/${artifact_name}?job=${job_name}"

    if [[ -f ${artifact_path} ]] && [[ "$(file -b --mime-type "${artifact_path}")" == "application/zip" ]]; then
        # TODO: handle error output if package_api is not reachable
        curl -su "${admin_user}:${admin_password}" --data-binary "@${artifact_path}" "${package_api_url}/admin/upload" | jq -r '.message'
    else
        echo "Did not find artifact \"${artifact_name}\" for ${tag} on Gitlab"
    fi
done
# }}}

# Cleanup {{{
echo "Cleaning up local archives"
rm -r "${download_dir}"
# }}}
