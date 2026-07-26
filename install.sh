#!/usr/bin/env sh
set -eu

repo="github.com/freddyrosa16/clibox"
bin_name="clibox"
min_go_minor="25"
min_go_patch="11"

expand_user_path() {
  case "$1" in
    "~")
      printf '%s\n' "$HOME"
      ;;
    "~/"*)
      printf '%s/%s\n' "$HOME" "${1#~/}"
      ;;
    *)
      printf '%s\n' "$1"
      ;;
  esac
}

path_has_dir() {
  case ":${PATH:-}:" in
    *":$1:"*) return 0 ;;
    *) return 1 ;;
  esac
}

shell_profile_path() {
  shell_name="${SHELL##*/}"
  case "$shell_name" in
    zsh)
      printf '%s\n' "$HOME/.zshrc"
      ;;
    bash)
      printf '%s\n' "$HOME/.bashrc"
      ;;
    *)
      printf '%s\n' "$HOME/.profile"
      ;;
  esac
}

ensure_install_dir_on_path() {
  install_dir="$1"
  if path_has_dir "$install_dir"; then
    return 0
  fi
  if [ "${CLIBOX_NO_PATH_UPDATE:-}" = "1" ]; then
    return 1
  fi

  profile="$(shell_profile_path)"
  line="export PATH=\"$install_dir:\$PATH\""
  if [ -f "$profile" ] && grep -F "$line" "$profile" >/dev/null 2>&1; then
    return 2
  fi
  {
    echo
    echo "# clibox"
    echo "$line"
  } >>"$profile" || return 1
  return 2
}

choose_install_dir() {
  if [ -n "${CLIBOX_INSTALL_DIR:-}" ]; then
    expand_user_path "$CLIBOX_INSTALL_DIR"
    return 0
  fi

  gopath_bin="$(go env GOPATH 2>/dev/null)/bin"
  for dir in "$HOME/.local/bin" "$HOME/bin" "$gopath_bin" /opt/homebrew/bin /usr/local/bin; do
    if [ -n "$dir" ] && [ -d "$dir" ] && [ -w "$dir" ] && path_has_dir "$dir"; then
      printf '%s\n' "$dir"
      return 0
    fi
  done

  printf '%s\n' "$HOME/.local/bin"
}

load_homebrew_path() {
  if command -v brew >/dev/null 2>&1; then
    return 0
  fi

  for brew_path in /opt/homebrew/bin/brew /usr/local/bin/brew /home/linuxbrew/.linuxbrew/bin/brew; do
    if [ -x "$brew_path" ]; then
      eval "$("$brew_path" shellenv)"
      return 0
    fi
  done

  return 1
}

install_homebrew() {
  if load_homebrew_path; then
    return 0
  fi

  if [ "${CLIBOX_INSTALL_HOMEBREW:-}" != "1" ]; then
    echo "Homebrew is required to install missing clibox dependencies automatically." >&2
    echo "Install Homebrew first: https://brew.sh/" >&2
    echo "Or explicitly allow this installer to run Homebrew's official installer:" >&2
    echo "  curl -fsSL https://raw.githubusercontent.com/freddyrosa16/clibox/main/install.sh | CLIBOX_INSTALL_HOMEBREW=1 sh" >&2
    exit 1
  fi

  case "$(uname -s)" in
    Darwin|Linux) ;;
    *)
      echo "Homebrew install is supported on macOS and Linux." >&2
      echo "Install the email backend manually: https://github.com/pimalaya/himalaya" >&2
      exit 1
      ;;
  esac

  if ! command -v curl >/dev/null 2>&1; then
    echo "Installing Homebrew requires curl." >&2
    exit 1
  fi
  if [ ! -x /bin/bash ]; then
    echo "Installing Homebrew requires /bin/bash." >&2
    exit 1
  fi

  echo "Homebrew was not found. Installing Homebrew..."
  /bin/bash -c "$(curl -fsSL https://raw.githubusercontent.com/Homebrew/install/HEAD/install.sh)"

  if ! load_homebrew_path; then
    echo "Homebrew installed, but brew is still not available in PATH." >&2
    echo "Restart your shell, then run this installer again." >&2
    exit 1
  fi
}

go_version_ok() {
  if ! command -v go >/dev/null 2>&1; then
    return 1
  fi

  version="$(go env GOVERSION 2>/dev/null || true)"
  version="${version#go}"
  major="${version%%.*}"
  rest="${version#*.}"
  minor="${rest%%.*}"
  minor="${minor%%[!0-9]*}"
  patch="${rest#*.}"
  if [ "$patch" = "$rest" ]; then
    patch="0"
  else
    patch="${patch%%[!0-9]*}"
  fi

  case "$major:$minor:$patch" in
    *[!0-9:]*|:|*:|*::)
      return 1
      ;;
  esac

  [ "$major" -gt 1 ] ||
    { [ "$major" -eq 1 ] && [ "$minor" -gt "$min_go_minor" ]; } ||
    { [ "$major" -eq 1 ] && [ "$minor" -eq "$min_go_minor" ] && [ "$patch" -ge "$min_go_patch" ]; }
}

install_go() {
  if go_version_ok; then
    return 0
  fi

  install_homebrew
  echo "Installing Go 1.${min_go_minor}.${min_go_patch}+ with Homebrew..."
  brew install go

  if ! go_version_ok; then
    echo "clibox install requires Go 1.${min_go_minor}.${min_go_patch} or newer." >&2
    echo "Install Go first: https://go.dev/dl/" >&2
    exit 1
  fi
}

install_himalaya() {
  if command -v himalaya >/dev/null 2>&1; then
    return 0
  fi

  install_homebrew
  echo "Installing email backend with Homebrew..."
  brew install himalaya
}

if [ "${CLIBOX_SKIP_HIMALAYA:-}" = "1" ]; then
  echo "Skipping Himalaya compatibility backend because CLIBOX_SKIP_HIMALAYA=1."
else
  install_himalaya
fi
install_go

install_dir="$(choose_install_dir)"
mkdir -p "$install_dir"
install_dir="$(cd "$install_dir" && pwd -P)"

echo "Installing ${bin_name} from ${repo} main..."
GOBIN="$install_dir" go install "${repo}@main"

echo
echo "Installed ${bin_name} to ${install_dir}/${bin_name}"
if [ "${CLIBOX_SKIP_HIMALAYA:-}" = "1" ]; then
  echo "Native backend is built in. Himalaya compatibility backend was skipped."
else
  echo "Himalaya compatibility backend is available. Native backend is built in."
fi
if path_has_dir "$install_dir"; then
  echo "Run it from any directory with:"
  echo "  ${bin_name}"
else
  if [ "${CLIBOX_NO_PATH_UPDATE:-}" = "1" ]; then
    echo "${install_dir} is not on your shell PATH."
  else
    if ensure_install_dir_on_path "$install_dir"; then
      path_status=0
    else
      path_status=$?
    fi
    if [ "$path_status" = "2" ]; then
      echo "Configured ${install_dir} on your shell PATH for future terminals."
    else
      echo "${install_dir} is not on your shell PATH, and the installer could not update it automatically."
    fi
  fi
  echo "Use it now with:"
  echo "  export PATH=\"${install_dir}:\$PATH\""
  echo "  ${bin_name}"
fi
echo "Check your email setup with:"
echo "  ${bin_name} doctor"
echo "Native account commands:"
echo "  ${bin_name} auth add --email you@gmail.com --account gmail"
echo "  ${bin_name} auth login --account gmail"
echo "  ${bin_name} --mail-backend native --account gmail"
