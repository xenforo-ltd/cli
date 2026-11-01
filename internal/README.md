# Package boundaries

`cmd/xf` defines the Cobra command tree, binds flags, and handles process-level
dispatch, signals, errors, and exit codes. It delegates execution to
`commands/<command>` using contexts, arguments, and concrete options.

Command packages do not import one another. Each subcommand has its own action
file in its parent's package. Helpers used by multiple actions belong in
separate responsibility-named files. Code used by different command families
belongs outside `commands`.

## Shared responsibilities

| Package | Responsibility |
| --- | --- |
| `auth` | OAuth and credential-source selection, storage, and token lifecycle. |
| `build` | Add-on release and JavaScript build execution. |
| `cache` | Downloaded archives, metadata, integrity checks, and cache storage. |
| `clierror` | Error classification, usage markers, hints, cancellation, and child exit status. |
| `config` | CLI configuration and its defaults. |
| `customerapi` | Authenticated access to the XenForo customer API. |
| `devoutput` | Conversion of prepared development output to release XML. |
| `downloads` | Licensed product selection, acquisition, and cached archive extraction. |
| `environment` | Environment files, Compose operations, and host/container execution. |
| `extract` | Safe extraction of XenForo archives. |
| `passthrough` | Optional installation arguments and shared tool invocation semantics. |
| `setup` | Environment setup reused by initialization and worktree creation. |
| `stream` | Stream progress reporting. |
| `ui` | Terminal rendering, status output, and subprocess progress presentation. |
| `version` | CLI build metadata, including the existing linker-injected values. |
| `xf` | Installation discovery, installed XenForo versions, and installation metadata. |

Export APIs for actual production consumers outside the package, not to expose
implementation details to tests. Serialization and interface methods retain the
visibility their protocols require. Existing tests live with the code they test;
`testutils` is explicitly test support.
