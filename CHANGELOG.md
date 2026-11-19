# Changelog

## [0.4.1] - 2026-11-19

### Changed

- Updated `golang.org/x/crypto` to v0.59.0.
- Updated the Kubernetes client libraries to v0.37.2
  (`k8s.io/client-go`, `k8s.io/api` and `k8s.io/apimachinery`).

## [0.4.0] - 2026-11-19

### Added

- A host and session sidebar: `LOCAL` and saved SSH, Docker and Kubernetes
  hosts in one flat list, with sessions as children and a search field.
- A new-session popover with icons, addresses, session counts and a New Host
  action.
- A right sidebar of files for local, SSH, Docker and Kubernetes sessions,
  with a Follow toggle for the shell's current directory.
- Docker container selection with search, image and state, and Kubernetes
  selection with searchable namespace, pod and container fields.
- Kubernetes kubeconfig and context support beside server and token.
- Program Status Protocol (OSC 7501), with status icons in session rows and
  messages on hover.
- Shortcut command previews, sixteen icons and a delete badge on hover.
- Signed automatic updates from GitHub releases, with Help > Check for
  Updates….
- English and Simplified Chinese READMEs, and updated screenshots.

### Fixed

- SSH input was not echoed back.
- SSH terminal width and height were swapped on resize.
- Docker and Kubernetes selection dialogs did not open.
- Docker, Kubernetes and SSH file sidebars now follow the shell's directory.
- The application icon now follows the macOS icon grid with transparent
  margins.
