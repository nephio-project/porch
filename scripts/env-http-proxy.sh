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
  echo "Usage: $0 [options]"
  echo
  echo "Options"
  echo "  -h, --help                    Show this help message and exit"
  echo "  --test-blueprints REPO_URL    Set the env URL for test-blueprints"
  echo "    --user=USER                 (Optional) Set the env USER for test-blueprints repository"
  echo "    --password=PASSWORD         (Optional) Set the env PASSWORD for test-blueprints"
  echo "  --gcp-blueprints REPO_URL     Set the env URL for gcp-blueprints repository"
  echo "    --user=USER                 (Optional) Set the env USER for gcp-blueprints"
  echo "    --password=PASSWORD         (Optional) Set the env PASSWORD for gcp-blueprints"
  echo "  --kpt-functions REPO_URL      Set the env URL for kpt-functions repository"
  echo "    --user=USER                 (Optional) Set the env USER for kpt-functions"
  echo "    --password=PASSWORD         (Optional) Set the env PASSWORD for kpt-functions"
  echo "  -u, --unset                   Unset all environment variables set by this script"
  echo
  echo "Example"
  echo "$0 --test-blueprints githuburl user=my_user password=my_password --gcp-blueprints githuburl user=my_user password=my_password"
}

PORCH_TEST_BLUEPRINTS_REPO_URL=""
PORCH_TEST_BLUEPRINTS_REPO_USER=""
PORCH_TEST_BLUEPRINTS_REPO_PASSWORD=""

PORCH_GCP_BLUEPRINTS_REPO_URL=""
PORCH_GCP_BLUEPRINTS_REPO_USER=""
PORCH_GCP_BLUEPRINTS_REPO_PASSWORD=""

PORCH_KPT_FUNCTIONS_REPO_URL=""
PORCH_KPT_FUNCTIONS_REPO_USER=""
PORCH_KPT_FUNCTIONS_REPO_PASSWORD=""

OPTIONS=$(getopt -o h --long help,test-blueprints:,gcp-blueprints:,kpt-functions:,user::,password:: -- "$@")

if [ $? -ne 0 ]; then
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
      export PORCH_KPT_FUNCTIONS_REPO_URL="$2"
      shift 2
      if [ "$1" == "--user" ]; then
        export PORCH_KPT_FUNCTIONS_REPO_USER="$2"
        shift 2
      fi
      if [ "$1" == "--password" ]; then
        export PORCH_KPT_FUNCTIONS_PASSWORD="$2"
        shift 2
      fi
      ;;
    '-u'|'--unset')
      export PORCH_TEST_BLUEPRINTS_REPO_URL=""
      export PORCH_TEST_BLUEPRINTS_REPO_USER=""
      export PORCH_TEST_BLUEPRINTS_REPO_PASSWORD=""
      export PORCH_GCP_BLUEPRINTS_REPO_URL=""
      export PORCH_GCP_BLUEPRINTS_REPO_USER=""
      export PORCH_GCP_BLUEPRINTS_REPO_PASSWORD=""
      export PORCH_KPT_FUNCTIONS_REPO_URL=""
      export PORCH_KPT_FUNCTIONS_REPO_USER=""
      export PORCH_KPT_FUNCTIONS_REPO_PASSWORD=""
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