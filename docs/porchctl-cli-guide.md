## Using the porchctl cli

When Porch was ported to Nephio, the `kpt alpha rpkg` commands in kpt were moved into a new command called `porchctl`. 

See the [project releases](https://github.com/nephio-project/porch/releases) for information on the latest release.

Once the preferred binary is downloaded and extracted, we can set it up for use locally.

_optional: Configure `porchctl` on your environment_

1. Either add `porchctl` to your PATH or copy it to a directory already in the path. 
```
cp porchctl /usr/local/bin/
```
2. Generate the autocompletion script for the specified shell to add to your profile.
```
porchctl completion bash
```

Check that porchctl is working:

```
porchctl --help

porchctl interacts with a Kubernetes API server with the Porch
server installed as an aggregated API server. It allows you to
manage Porch repository registrations and the packages within
those repositories.

Usage:
  porchctl [flags]
  porchctl [command]

Available Commands:
  completion  Generate the autocompletion script for the specified shell
  help        Help about any command
  repo        Manage package repositories.
  rpkg        Manage packages.
  version     Print the version number of porchctl

Flags:
  -h, --help                           help for porchctl
      --log-flush-frequency duration   Maximum number of seconds between log flushes (default 5s)
      --truncate-output                Enable the truncation for output (default true)
  -v, --v Level                        number for the log level verbosity

Use "porchctl [command] --help" for more information about a command.

```

The `porchtcl` command is an administration command for acting on Porch `Repository` (repo) and `PackageRevision` (rpkg) CRs.

The commands for administering repositories are:

| Command               | Description                    |
| --------------------- | ------------------------------ |
| `porchctl repo get`   | List registered repositories.  |
| `porchctl repo reg`   | Register a package repository. |
| `porchctl repo unreg` | Unregister a repository.       |

The commands for administering package revisions are:

| Command                        | Description                                                                             |
| ------------------------------ | --------------------------------------------------------------------------------------- |
| `porchctl rpkg approve`        | Approve a proposal to publish a package revision.                                       |
| `porchctl rpkg clone`          | Create a clone of an existing package revision.                                         |
| `porchctl rpkg copy`           | Create a new package revision from an existing one.                                     |
| `porchctl rpkg del`            | Delete a package revision.                                                              |
| `porchctl rpkg get`            | List package revisions in registered repositories.                                      |
| `porchctl rpkg init`           | Initializes a new package in a repository.                                              |
| `porchctl rpkg propose`        | Propose that a package revision should be published.                                    |
| `porchctl rpkg propose-delete` | Propose deletion of a published package revision.                                       |
| `porchctl rpkg pull`           | Pull the content of the package revision.                                               |
| `porchctl rpkg push`           | Push resources to a package revision.                                                   |
| `porchctl rpkg reject`         | Reject a proposal to publish or delete a package revision.                              |
| `porchctl rpkg update`         | Update a downstream package revision to a more recent revision of its upstream package. |
