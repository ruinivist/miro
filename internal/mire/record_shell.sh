#!/bin/sh
set -eu

host_home=${MIRE_HOST_HOME:?}
host_tmp=${MIRE_HOST_TMP:?}
path_env=${MIRE_PATH_ENV:?}
visible_home=${MIRE_HOME:?}
bootstrap_rc="$host_home/.mire-shell-rc"
visible_bootstrap_rc="$visible_home/.mire-shell-rc"
setup_scripts_dir='/tmp/mire-setup-scripts'
visible_bin_dir='/tmp/mire/bin'

cat >"$bootstrap_rc" <<'EOF'
cd "${HOME:?}"

for path in /tmp/mire-setup-scripts/*.sh; do
  [ -e "$path" ] || continue
  cd "${HOME:?}"
  source "$path"
  cd "${HOME:?}"
done

if [ "${MIRE_COMPARE_MARKER:-0}" = "1" ]; then
  __mire_prompt_ready_original=${PROMPT_COMMAND-}
  __mire_prompt_ready() {
    printf '__MIRE_PROMPT_READY__\n'
    if [ -n "${__mire_prompt_ready_original:-}" ]; then
      PROMPT_COMMAND=$__mire_prompt_ready_original
      eval "$PROMPT_COMMAND"
    else
      unset PROMPT_COMMAND
    fi
  }
  PROMPT_COMMAND=__mire_prompt_ready
fi
EOF

# the first ro-bind allows for /usr/bin etc to be mounted and accessible
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
  --setenv PATH "$visible_bin_dir:$path_env" \
  --setenv PS1 '$ ' \
  --setenv TERM 'xterm-256color' \
  --setenv TMPDIR '/tmp' \
  --setenv TZ 'UTC' \
  --chdir "$visible_home"

set -- "$@" --dir /tmp/mire --dir "$visible_bin_dir"

if [ -n "${MIRE_SETUP_SCRIPTS:-}" ]; then
  i=1
  while IFS= read -r host_path || [ -n "$host_path" ]; do
    [ -n "$host_path" ] || continue
    visible_path=$(printf '%s/%03d.sh' "$setup_scripts_dir" "$i")
    set -- "$@" --ro-bind "$host_path" "$visible_path"
    i=$((i + 1))
  done <<EOF
${MIRE_SETUP_SCRIPTS-}
EOF
fi

if [ -n "${MIRE_MOUNTS:-}" ]; then
  while IFS= read -r mount || [ -n "$mount" ]; do
    [ -n "$mount" ] || continue
    host_path=${mount%%:*}
    sandbox_path=${mount#*:}
    set -- "$@" --ro-bind "$host_path" "$sandbox_path"
  done <<EOF
${MIRE_MOUNTS-}
EOF
fi

if [ -n "${MIRE_PATHS:-}" ]; then
  while IFS= read -r host_path || [ -n "$host_path" ]; do
    [ -n "$host_path" ] || continue
    visible_path=$visible_bin_dir/${host_path##*/}
    set -- "$@" --ro-bind "$host_path" "$visible_path"
  done <<EOF
${MIRE_PATHS-}
EOF
fi

exec bwrap "$@" bash --noprofile --rcfile "$visible_bootstrap_rc" -i
