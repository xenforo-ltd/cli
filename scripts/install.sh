#!/usr/bin/env bash
set -euo pipefail

APP="xf"
REPO="xenforo-ltd/xf"
INSTALL_DIR="$HOME/.xf/bin"

MIN_GIT_VERSION="2.25.1"
MIN_DOCKER_VERSION="20.10.0"

RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[0;33m'
NC='\033[0m'

usage() {
    cat <<EOF
XenForo CLI Installer

Usage: install.sh [options]

Options:
    -h, --help              Display this help message
    -v, --version <version> Install a specific version (e.g., 1.0.0)
    -b, --binary <path>     Install from a local binary instead of downloading
        --no-modify-path    Don't modify shell config files (.zshrc, .bashrc, etc.)
        --skip-prereq       Skip prerequisite checks

Examples:
    curl -fsSL https://raw.githubusercontent.com/$REPO/main/scripts/install.sh | bash
    curl -fsSL https://raw.githubusercontent.com/$REPO/main/scripts/install.sh | bash -s -- --version 1.0.0
    ./install.sh --binary /path/to/xf
EOF
}

requested_version=""
no_modify_path=false
skip_prereq=false
binary_path=""

while [[ $# -gt 0 ]]; do
    case "$1" in
        -h|--help)
            usage
            exit 0
            ;;
        -v|--version)
            if [[ -n "${2:-}" ]]; then
                requested_version="$2"
                shift 2
            else
                echo -e "${RED}Error: --version requires a version argument${NC}"
                exit 1
            fi
            ;;
        -b|--binary)
            if [[ -n "${2:-}" ]]; then
                binary_path="$2"
                shift 2
            else
                echo -e "${RED}Error: --binary requires a path argument${NC}"
                exit 1
            fi
            ;;
        --no-modify-path)
            no_modify_path=true
            shift
            ;;
        --skip-prereq)
            skip_prereq=true
            shift
            ;;
        *)
            echo -e "${YELLOW}Warning: Unknown option '$1'${NC}" >&2
            shift
            ;;
    esac
done

version_gte() {
    local v1="$1"
    local v2="$2"
    
    if printf '%s\n%s\n' "$v2" "$v1" | sort -V -C 2>/dev/null; then
        return 0
    fi
    
    local IFS='.'
    local i v1_parts=($v1) v2_parts=($v2)
    
    for ((i=0; i<${#v2_parts[@]}; i++)); do
        local v1_part="${v1_parts[i]:-0}"
        local v2_part="${v2_parts[i]:-0}"
        
        v1_part="${v1_part%%[!0-9]*}"
        v2_part="${v2_part%%[!0-9]*}"
        
        if ((v1_part > v2_part)); then
            return 0
        elif ((v1_part < v2_part)); then
            return 1
        fi
    done
    
    return 0
}

check_prerequisites() {
    local has_errors=false
    local missing_deps=()
    local version_errors=()
    
    echo "Checking prerequisites..."
    
    if command -v curl >/dev/null 2>&1; then
        echo -e "  ${GREEN}✓${NC} curl"
    elif command -v wget >/dev/null 2>&1; then
        echo -e "  ${GREEN}✓${NC} wget"
    else
        echo -e "  ${RED}✗${NC} curl or wget not found"
        missing_deps+=("curl")
        has_errors=true
    fi
    
    if command -v tar >/dev/null 2>&1; then
        echo -e "  ${GREEN}✓${NC} tar"
    else
        echo -e "  ${RED}✗${NC} tar not found"
        missing_deps+=("tar")
        has_errors=true
    fi
    
    if command -v git >/dev/null 2>&1; then
        local git_version
        git_version=$(git --version | sed -n 's/git version \([0-9.]*\).*/\1/p')
        if version_gte "$git_version" "$MIN_GIT_VERSION"; then
            echo -e "  ${GREEN}✓${NC} git $git_version"
        else
            echo -e "  ${RED}✗${NC} git $git_version (requires >= $MIN_GIT_VERSION)"
            version_errors+=("git")
            has_errors=true
        fi
    else
        echo -e "  ${RED}✗${NC} git not found"
        missing_deps+=("git")
        has_errors=true
    fi
    
    if command -v docker >/dev/null 2>&1; then
        local docker_version
        docker_version=$(docker --version | sed -n 's/Docker version \([0-9.]*\).*/\1/p')
        if version_gte "$docker_version" "$MIN_DOCKER_VERSION"; then
            echo -e "  ${GREEN}✓${NC} docker $docker_version"
            
            if docker compose version >/dev/null 2>&1; then
                local compose_version
                compose_version=$(docker compose version --short 2>/dev/null || echo "unknown")
                echo -e "  ${GREEN}✓${NC} docker compose $compose_version"
            else
                echo -e "  ${RED}✗${NC} docker compose not available"
                version_errors+=("docker-compose")
                has_errors=true
            fi
        else
            echo -e "  ${RED}✗${NC} docker $docker_version (requires >= $MIN_DOCKER_VERSION)"
            version_errors+=("docker")
            has_errors=true
        fi
    else
        echo -e "  ${RED}✗${NC} docker not found"
        missing_deps+=("docker")
        has_errors=true
    fi
    
    if [[ "$has_errors" == "true" ]]; then
        echo ""
        echo -e "${RED}Missing or outdated dependencies:${NC}"
        echo ""
        
        for dep in "${missing_deps[@]:-}"; do
            case "$dep" in
                curl)
                    echo -e "${YELLOW}curl${NC} is required to download files."
                    echo "  macOS:   brew install curl"
                    echo "  Ubuntu:  sudo apt install curl"
                    echo ""
                    ;;
                tar)
                    echo -e "${YELLOW}tar${NC} is required to extract archives."
                    echo "  Usually pre-installed on Unix systems."
                    echo ""
                    ;;
                git)
                    echo -e "${YELLOW}Git${NC} is required for XenForo CLI workflows."
                    echo "  macOS:   brew install git"
                    echo "  Ubuntu:  sudo apt install git"
                    echo "  More:    https://git-scm.com/downloads"
                    echo ""
                    ;;
                docker)
                    echo -e "${YELLOW}Docker${NC} is required to run XenForo development environments."
                    echo "  macOS:   https://docs.docker.com/desktop/install/mac-install/"
                    echo "  Linux:   https://docs.docker.com/engine/install/"
                    echo "  Windows: https://docs.docker.com/desktop/install/windows-install/"
                    echo ""
                    ;;
            esac
        done
        
        for dep in "${version_errors[@]:-}"; do
            case "$dep" in
                git)
                    echo -e "${YELLOW}Git${NC} version $MIN_GIT_VERSION or later is required."
                    echo "  Please update Git: https://git-scm.com/downloads"
                    echo ""
                    ;;
                docker)
                    echo -e "${YELLOW}Docker${NC} version $MIN_DOCKER_VERSION or later is required."
                    echo "  Please update Docker: https://docs.docker.com/engine/install/"
                    echo ""
                    ;;
                docker-compose)
                    echo -e "${YELLOW}Docker Compose V2${NC} is required but not available."
                    echo "  Docker Compose V2 is included with Docker Desktop and recent Docker Engine versions."
                    echo "  Please update Docker: https://docs.docker.com/engine/install/"
                    echo ""
                    ;;
            esac
        done
        
        echo "Please install the missing dependencies and run this script again."
        echo "Or use --skip-prereq to bypass these checks (not recommended)."
        exit 1
    fi
    
    echo ""
}

detect_os() {
    local raw_os
    raw_os=$(uname -s)
    
    case "$raw_os" in
        Darwin*) echo "darwin" ;;
        Linux*)  echo "linux" ;;
        MINGW*|MSYS*|CYGWIN*)
            echo -e "${RED}Error: This script is for macOS/Linux. Please use install.ps1 for Windows.${NC}" >&2
            exit 1
            ;;
        *)
            echo -e "${RED}Error: Unsupported operating system: $raw_os${NC}" >&2
            exit 1
            ;;
    esac
}

detect_arch() {
    local arch
    arch=$(uname -m)
    
    case "$arch" in
        x86_64)  echo "amd64" ;;
        amd64)   echo "amd64" ;;
        aarch64) echo "arm64" ;;
        arm64)   echo "arm64" ;;
        *)
            echo -e "${RED}Error: Unsupported architecture: $arch${NC}" >&2
            exit 1
            ;;
    esac
}

check_rosetta() {
    local os="$1"
    local arch="$2"
    
    if [[ "$os" == "darwin" && "$arch" == "amd64" ]]; then
        local rosetta_flag
        rosetta_flag=$(sysctl -n sysctl.proc_translated 2>/dev/null || echo 0)
        if [[ "$rosetta_flag" == "1" ]]; then
            echo "arm64"
            return
        fi
    fi
    
    echo "$arch"
}

get_latest_version() {
    local url="https://api.github.com/repos/$REPO/releases/latest"
    local version
    
    if command -v curl >/dev/null 2>&1; then
        version=$(curl -s "$url" | sed -n 's/.*"tag_name": *"v\([^"]*\)".*/\1/p')
    else
        version=$(wget -qO- "$url" | sed -n 's/.*"tag_name": *"v\([^"]*\)".*/\1/p')
    fi
    
    if [[ -z "$version" ]]; then
        echo -e "${RED}Error: Failed to fetch latest version from GitHub${NC}" >&2
        exit 1
    fi
    
    echo "$version"
}

download_file() {
    local url="$1"
    local output="$2"
    
    echo "Downloading from: $url"
    
    if command -v curl >/dev/null 2>&1; then
        curl -#fSL -o "$output" "$url"
    else
        wget --progress=bar:force -O "$output" "$url"
    fi
}

verify_checksum() {
    local file="$1"
    local checksums_file="$2"
    local filename
    filename=$(basename "$file")
    
    local expected
    expected=$(awk -v target="$filename" '
        NF >= 2 {
            name = $2
            sub(/^\*/, "", name)
            if (name == target) {
                print $1
                exit
            }
        }
    ' "$checksums_file")
    
    if [[ -z "$expected" ]]; then
        echo -e "${RED}Error: No checksum entry found for $filename${NC}"
        exit 1
    fi
    
    local actual
    if command -v sha256sum >/dev/null 2>&1; then
        actual=$(sha256sum "$file" | awk '{print $1}')
    elif command -v shasum >/dev/null 2>&1; then
        actual=$(shasum -a 256 "$file" | awk '{print $1}')
    else
        echo -e "${YELLOW}Warning: No SHA256 tool available, skipping verification${NC}"
        return 0
    fi
    
    if [[ "$expected" != "$actual" ]]; then
        echo -e "${RED}Error: Checksum verification failed${NC}"
        echo "  Expected: $expected"
        echo "  Actual:   $actual"
        exit 1
    fi
    
    echo -e "  ${GREEN}✓${NC} Checksum verified"
}

check_existing() {
    local version="$1"
    
    if command -v xf >/dev/null 2>&1; then
        local installed_version
        installed_version=$(xf version --short 2>/dev/null || echo "")
        
        if [[ "$installed_version" == "v$version" || "$installed_version" == "$version" ]]; then
            echo "Version $version is already installed"
            exit 0
        fi
        
        if [[ -n "$installed_version" ]]; then
            echo "Installed version: $installed_version"
        fi
    fi
}

add_to_path() {
    local config_file="$1"
    local command="$2"
    
    if grep -Fxq "$command" "$config_file" 2>/dev/null; then
        echo "PATH already configured in $config_file"
    elif [[ -w "$config_file" ]]; then
        echo "" >> "$config_file"
        echo "# xf" >> "$config_file"
        echo "$command" >> "$config_file"
        echo "Added to PATH in $config_file"
    else
        echo -e "${YELLOW}Please add the following to your shell config:${NC}"
        echo "  $command"
    fi
}

update_path() {
    if [[ "$no_modify_path" == "true" ]]; then
        return
    fi
    
    if [[ ":$PATH:" == *":$INSTALL_DIR:"* ]]; then
        return
    fi
    
    local current_shell
    current_shell=$(basename "$SHELL")
    
    local config_file=""
    
    case "$current_shell" in
        fish)
            config_file="$HOME/.config/fish/config.fish"
            if [[ -f "$config_file" ]]; then
                add_to_path "$config_file" "fish_add_path $INSTALL_DIR"
            fi
            ;;
        zsh)
            for f in "${ZDOTDIR:-$HOME}/.zshrc" "$HOME/.zshrc"; do
                if [[ -f "$f" ]]; then
                    config_file="$f"
                    break
                fi
            done
            if [[ -n "$config_file" ]]; then
                add_to_path "$config_file" "export PATH=\"$INSTALL_DIR:\$PATH\""
            fi
            ;;
        bash)
            for f in "$HOME/.bashrc" "$HOME/.bash_profile" "$HOME/.profile"; do
                if [[ -f "$f" ]]; then
                    config_file="$f"
                    break
                fi
            done
            if [[ -n "$config_file" ]]; then
                add_to_path "$config_file" "export PATH=\"$INSTALL_DIR:\$PATH\""
            fi
            ;;
        *)
            echo -e "${YELLOW}Please add $INSTALL_DIR to your PATH manually${NC}"
            ;;
    esac
    
    if [[ -n "${GITHUB_ACTIONS:-}" && "${GITHUB_ACTIONS}" == "true" ]]; then
        echo "$INSTALL_DIR" >> "$GITHUB_PATH"
        echo "Added to \$GITHUB_PATH"
    fi
}

main() {
    if [[ "$skip_prereq" != "true" && -z "$binary_path" ]]; then
        check_prerequisites
    fi
    
    mkdir -p "$INSTALL_DIR"
    
    if [[ -n "$binary_path" ]]; then
        if [[ ! -f "$binary_path" ]]; then
            echo -e "${RED}Error: Binary not found at $binary_path${NC}"
            exit 1
        fi
        
        echo -e "Installing ${APP} from local binary..."
        cp "$binary_path" "$INSTALL_DIR/$APP"
        chmod 755 "$INSTALL_DIR/$APP"
        
        update_path
        
        echo ""
        echo -e "${GREEN}Successfully installed $APP${NC}"
        echo ""
        echo "To get started:"
        echo "  1. Restart your shell or run: export PATH=\"$INSTALL_DIR:\$PATH\""
        echo "  2. Run: xf auth login"
        echo "  3. Run: xf init ./my-project"
        echo ""
        exit 0
    fi
    
    local os arch
    os=$(detect_os)
    arch=$(detect_arch)
    arch=$(check_rosetta "$os" "$arch")
    
    echo "Detected platform: $os/$arch"
    
    local version
    if [[ -n "$requested_version" ]]; then
        version="${requested_version#v}"
        
        local check_url="https://github.com/$REPO/releases/tag/v$version"
        local http_status
        if command -v curl >/dev/null 2>&1; then
            http_status=$(curl -sI -o /dev/null -w "%{http_code}" "$check_url")
        else
            http_status=$(wget --spider -S "$check_url" 2>&1 | grep "HTTP/" | tail -1 | awk '{print $2}')
        fi
        
        if [[ "$http_status" == "404" ]]; then
            echo -e "${RED}Error: Release v$version not found${NC}"
            echo "Available releases: https://github.com/$REPO/releases"
            exit 1
        fi
    else
        echo "Fetching latest version..."
        version=$(get_latest_version)
    fi
    
    echo -e "Installing ${APP} version ${GREEN}v$version${NC}"
    
    check_existing "$version"
    
    local tmp_dir
    tmp_dir=$(mktemp -d)
    trap "rm -rf '$tmp_dir'" EXIT
    
    local archive_name="xf-v$version-$os-$arch.tar.gz"
    local download_url="https://github.com/$REPO/releases/download/v$version/$archive_name"
    local checksums_url="https://github.com/$REPO/releases/download/v$version/checksums.txt"
    
    download_file "$download_url" "$tmp_dir/$archive_name"
    download_file "$checksums_url" "$tmp_dir/checksums.txt"
    
    echo "Verifying checksum..."
    verify_checksum "$tmp_dir/$archive_name" "$tmp_dir/checksums.txt"
    
    echo "Extracting..."
    tar -xzf "$tmp_dir/$archive_name" -C "$tmp_dir"
    
    mv "$tmp_dir/$APP" "$INSTALL_DIR/$APP"
    chmod 755 "$INSTALL_DIR/$APP"
    
    update_path
    
    echo ""
    echo -e "${GREEN}Successfully installed $APP v$version${NC}"
    echo ""
    echo "To get started:"
    echo "  1. Restart your shell or run: export PATH=\"$INSTALL_DIR:\$PATH\""
    echo "  2. Run: xf auth login"
    echo "  3. Run: xf init ./my-project"
    echo ""
}

main
