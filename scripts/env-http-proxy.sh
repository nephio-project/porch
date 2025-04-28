# Copyright 2025 The kpt and Nephio Authors
#
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
#      http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.

usage() {
  cat << EOF
Usage: $0 [options]"

Options"
  -h, --help                    Show this help message and exit
  --test-blueprints REPO_URL    Set the env URL for test-blueprints
    --user=USER                 (Optional) Set the env USER for test-blueprints repository
    --password=PASSWORD         (Optional) Set the env PASSWORD for test-blueprints
  --gcp-blueprints REPO_URL     Set the env URL for gcp-blueprints repository
    --user=USER                 (Optional) Set the env USER for gcp-blueprints
    --password=PASSWORD         (Optional) Set the env PASSWORD for gcp-blueprints
  --kpt-functions REPO_URL      Set the env URL for kpt-functions repository
    --user=USER                 (Optional) Set the env USER for kpt-functions
    --password=PASSWORD         (Optional) Set the env PASSWORD for kpt-functions
  --gcr-io-prefix PREFIX        Set the env URL for pulling gcr-io images
  -u, --unset                   Unset all environment variables set by this script

Example
  source $0 --test-blueprints githuburl user=my_user password=my_password --gcp-blueprints githuburl user=my_user password=my_password
EOF
}

PORCH_TEST_BLUEPRINTS_REPO_URL=""
PORCH_TEST_BLUEPRINTS_REPO_USER=""
PORCH_TEST_BLUEPRINTS_REPO_PASSWORD=""

PORCH_GCP_BLUEPRINTS_REPO_URL=""
PORCH_GCP_BLUEPRINTS_REPO_USER=""
PORCH_GCP_BLUEPRINTS_REPO_PASSWORD=""

PORCH_KPT_REPO_URL=""
PORCH_KPT_REPO_USER=""
PORCH_KPT_REPO_PASSWORD=""

PORCH_GCR_PREFIX_URL=""

OPTIONS=$(getopt -o h,u --long help,test-blueprints:,gcp-blueprints:,kpt-functions:,user::,password::,gcr-io-prefix:,unset -- "$@")

if [ $# -le 0 ]; then
  usage
  exit 1
fi

eval set -- "$OPTIONS"

while true; do
  case $1 in
    '-h'|'--help')
      usage
      exit 0
      ;;
    '--test-blueprints')
      export PORCH_TEST_BLUEPRINTS_REPO_URL="$2"
      shift 2
      if [ "$1" == "--user" ]; then
        export PORCH_TEST_BLUEPRINTS_REPO_USER="$2"
        shift 2
      fi
      if [ "$1" == "--password" ]; then
        export PORCH_TEST_BLUEPRINTS_REPO_PASSWORD="$2"
        shift 2
      fi
      ;;
    '--gcp-blueprints')
      export PORCH_GCP_BLUEPRINTS_REPO_URL="$2"
      shift 2
      if [ "$1" == "--user" ]; then
        export PORCH_GCP_BLUEPRINTS_REPO_USER="$2"
        shift 2
      fi
      if [ "$1" == "--password" ]; then
        export PORCH_GCP_BLUEPRINTS_REPO_PASSWORD="$2"
        shift 2
      fi
      ;;
    '--kpt-functions')
      export PORCH_KPT_REPO_URL="$2"
      shift 2
      if [ "$1" == "--user" ]; then
        export PORCH_KPT_REPO_USER="$2"
        shift 2
      fi
      if [ "$1" == "--password" ]; then
        export PORCH_KPT_REPO_PASSWORD="$2"
        shift 2
      fi
      ;;
    '--gcr-io-prefix')
      export PORCH_GCR_PREFIX_URL="$2"
      shift 2
      ;;
    '-u'|'--unset')
      export PORCH_TEST_BLUEPRINTS_REPO_URL=""
      export PORCH_TEST_BLUEPRINTS_REPO_USER=""
      export PORCH_TEST_BLUEPRINTS_REPO_PASSWORD=""
      export PORCH_GCP_BLUEPRINTS_REPO_URL=""
      export PORCH_GCP_BLUEPRINTS_REPO_USER=""
      export PORCH_GCP_BLUEPRINTS_REPO_PASSWORD=""
      export PORCH_KPT_REPO_URL=""
      export PORCH_KPT_REPO_USER=""
      export PORCH_KPT_REPO_PASSWORD=""
      export PORCH_GCR_PREFIX_URL=""
      break
      ;;
    --)
      shift
      break
    ;;
    *)
      echo 'Internal error!' >&2
      exit 1
  esac
done