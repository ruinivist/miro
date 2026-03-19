#!/bin/sh
set -eu

# hoisted vars to fail fast if any missing
host_home=${MIRO_HOST_HOME:?}
host_tmp=${MIRO_HOST_TMP:?}
path_env=${MIRO_PATH_ENV:?}
visible_home=${MIRO_VISIBLE_HOME:?}

if [ "${MIRO_COMPARE_MARKER:-0}" = "1" ]; then
  printf '__MIRO_E2E_BEGIN__\n'
fi

set -- \
  --ro-bind / / \
  --tmpfs /home \
  --bind "$host_home" "$visible_home" \
  --bind "$host_tmp" '/tmp' \
  --dev /dev \
  --proc /proc \
  --unshare-pid \
  --die-with-parent \
  --setenv HISTFILE '/dev/null' \
  --setenv HOME "$visible_home" \
  --setenv LANG 'C' \
  --setenv LC_ALL 'C' \
  --setenv PAGER 'cat' \
  --setenv PATH "$path_env" \
  --setenv PS1 '$ ' \
  --setenv TERM 'xterm-256color' \
  --setenv TMPDIR '/tmp' \
  --setenv TZ 'UTC' \
  --chdir "$visible_home"

exec bwrap "$@" bash --noprofile --norc -i
