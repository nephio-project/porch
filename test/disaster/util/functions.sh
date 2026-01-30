# Copyright 2026 The kpt and Nephio Authors
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
function h1() {
  MESSAGE=" $* "
  TERMINAL_WIDTH=$(tput -T xterm cols)
  STAR_COUNT=$((((TERMINAL_WIDTH - ${#MESSAGE}) / 2) - 2))
  STAR_SEGMENT="$(head -c "$STAR_COUNT" < /dev/zero | tr "\0" "-")"
  MESSAGE_LINE="$(tput setaf 4)$STAR_SEGMENT<$(tput sgr0)$MESSAGE$(tput setaf 4)>$STAR_SEGMENT$(tput sgr0)"
  echo
  echo -e "$MESSAGE_LINE"
  echo
}

function h2() {
  MESSAGE=" $* "
  MESSAGE_LINE="$(tput setaf 4)===\u200B===\u200B===\u200B===>>$(tput sgr0) $MESSAGE"
  echo
  echo -e "$MESSAGE_LINE"
}