# Changelog

## [0.6.0] - 2026-11-25

### Changed

- A remote branch's chip keeps its remote in its name, as `origin/main`, so
  it reads apart from the local branch of the same name.
- The app is built on MyGo 0.10.0, with the latest purego and SFTP
  libraries.

## [0.5.0] - 2026-11-20

### Added

- The right sidebar has two views, Files and Git. The Git view shows the
  branch, how far it is from its upstream, and the changed files with their
  state, untracked, modified, staged or conflicted. It asks this machine's
  git, or git on an SSH server, in a container or in a pod, and the chosen
  view is saved with the window's settings.
- A changed file of the Git view opens its changes in the pane on a double
  click, or with Show Changes: MyGo's diff viewer compares what HEAD holds
  with the file as it is now, in one column or side by side, and reads them
  again on Refresh.
- The Git view splits into two parts, a drag between them: the changes,
  with a box under them to write a commit's message and commit at a button,
  and the commits of the branch. Each change has a box, checked at first and
  turned off by a click: the commit takes only the checked files, staging
  those and leaving the rest behind, with Select all and a count beside the
  button. An untracked file's + adds it to Git, which then shows it as
  added.
- A Git Graph button in the Git view opens the repository's commits in the
  pane: lanes colored by branch, refs and tags as chips, each commit's short
  hash, and who made it and when in its tooltip.
- A commit, in the Git view or in the graph, is chosen by a click and opens
  on a double click: what it says, who made it, its hash, date and refs on
  top, and the diff of each file it changed below, in one scroll. A click on
  a file's name folds its diff away, and the diffs show side by side or one
  over the other, as every diff in the pane does.
- Files open as what they are: a Markdown file rendered, with a button that
  shows its source, a picture in a preview that fits the pane, and any other
  text in the editor, whose line wrapping a button turns on and off.
- The branch row of the Git view switches the repository's branch from a
  popover of them all, with a field that searches them, and checks the
  chosen one out.
- The commits of the Git view and of the graph filter by their subject,
  author and hash, or by author:name, msg:text and hash:abc; a count says
  how many the filter keeps, and the graph's lanes stop at the commits it
  leaves out.
- The file sidebar makes files and folders, renames them, deletes them and
  copies their paths: a `+` button in its header, and a right click on a
  row or on the list for New File, New Folder, Rename, Delete, Copy Path
  and Copy Name. It works on local, SSH, Docker and Kubernetes sessions.
- The middle area splits into the terminal and a pane beside it, whose tabs
  hold files in the editor and web pages. A drag on the pane's edge moves
  it, and closing a tab with unsaved changes asks first.
- The window has no title bar of its own: the traffic lights (macOS) or the
  minimize, maximize and close buttons sit at the top of the session list,
  which also holds the new session and new host buttons, and the room
  between them drags the window, with a strip of the wash above the cards,
  as beside and below them. With the sidebar hidden, the window controls
  show in the terminal's title bar instead, in the same place: toggling the
  sidebar moves none of them.
- The terminal has a title bar of its own: the sidebars' toggles sit at its
  ends, the session's name and host beside the left one, and the search by
  the right one, whose field keeps a modest width while finding, at the
  right of the bar with the match count and buttons. With the pane beside the terminal, the file sidebar's toggle
  moves to the pane's tabs, at their right. The card with no session says
  the app's name with both toggles, and the update reminder moved to the
  foot of the sidebar.
- Files open in a built-in code editor, MyGo's `ui/editor`: syntax
  highlighting, find and replace, several carets, folding and soft wraps.
  Save writes the text back where the file is, on this machine, an SSH
  server, a container or a pod, from Command+S or the File menu; the tab's
  button opens the file with the application the system has for it
  instead. This machine's files in the row's menu still open with that
  application, and files on the other side of a network open there as a
  copy here, up to 32 MB. Files that are not text, as images, do not open
  in the editor.
- Command+click (Control+click elsewhere) on a URL the terminal shows opens
  it in a web view in that pane, with back, forward, reload and open in the
  system's browser. A file path a command printed opens in the editor, at
  the file the `:line:col` suffix names. Where a web view cannot be made,
  the tab says so and opens the page in the system's browser instead; the
  Linux packages now depend on WebKitGTK for them.

### Changed

- The graph's rows leave more room to what a commit says: its refs show as
  short chips, two at most with the rest counted as +N, and the whole ref
  in their tooltip. A lane a commit already has joins it instead of ending
  in a stem.

### Fixed

- The Git view's splitter drags again: it read the pointer's sideways
  movement, which a vertical drag does not have.
- A changed file's diff compared HEAD with the file as it is now, rather
  than with what HEAD holds, so every file of the Git view looked added: git
  was given the file's path on this machine instead of its path in the
  repository.
- Dragging a sidebar's edge resizes it again: the width had snapped back
  to where the drag began within the frame each move arrived in, and it did
  not survive a restart.
- The gap between the host list and the terminal is the same as the one
  between the terminal and the file sidebar.

## [0.4.3] - 2026-11-19

### Added

- Six color themes with a small effect preview: Paper, Ocean, Forest,
  Sunset, Violet and Monochrome. The palette button at the foot of the host
  sidebar applies one at once, and the choice is saved.

### Changed

- The packaged application is now named `GoShell`, without the space. On
  first launch, settings and updater preferences move from the old
  `Go Shell` data directory to `GoShell`; the old copy stays.

## [0.4.2] - 2026-11-19

### Changed

- The Find bar now sits at the right of the title bar instead of above the
  terminal.
- The host sidebar and the central card no longer show a divider line.
- Sessions under a host line up with the host's icons and text, without an
  extra indent.
- About shows the running version and offers Check for Updates….
- A download reminder appears in the title bar when an update is available.
- The host sidebar's width is now saved between launches.

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
