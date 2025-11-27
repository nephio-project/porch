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