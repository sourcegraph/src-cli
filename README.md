# Sourcegraph CLI [![Build Status](https://travis-ci.org/sourcegraph/src-cli.svg)](https://travis-ci.org/sourcegraph/src-cli) [![Build status](https://ci.appveyor.com/api/projects/status/fwa1bkd198hyim8a?svg=true)](https://ci.appveyor.com/project/sourcegraph/src-cli) [![Go Report Card](https://goreportcard.com/badge/sourcegraph/src-cli)](https://goreportcard.com/report/sourcegraph/src-cli)

The Sourcegraph `src` CLI provides access to [Sourcegraph](https://sourcegraph.com) via a command-line interface.

![image](https://user-images.githubusercontent.com/3173176/43567326-3db5f31c-95e6-11e8-9e74-4c04079c01b0.png)

It currently provides the ability to:

- **Execute search queries** from the command line and get nice colorized output back (or JSON, optionally).
- **Execute GraphQL queries** against a Sourcegraph instance, and get JSON results back (`src api`).
  - You can provide your API access token via an environment variable or file on disk.
  - You can easily convert a `src api` command into a curl command with `src api -get-curl`.
- **Manage repositories, users, and organizations** using the `src repos`, `src users`, and `src orgs` commands.

If there is something you'd like to see Sourcegraph be able to do from the CLI, let us know! :)

## Installation

The preferred method of installation is to ask _your_ Sourcegraph instance for the latest compatible version. To do this, replace `https://sourcegraph.com` in the commands below with the address of your instance. If you are running a Sourcegraph version older than 3.12, these URLs will not be available and you must install directly from sourcegraph.com (by running the following commands _exactly_), or from one of the published [releases on GitHub](https://github.com/sourcegraph/src-cli/releases).

Check the releases page for the latest version (or a specific version), then follow the instructions for your operating system using the download URL:

```
https://github.com/sourcegraph/src-cli/releases/download/{version}/{binary}
````

### Mac OS

```bash
curl -L https://sourcegraph.com/.api/src-cli/src_darwin_amd64 -o /usr/local/bin/src
chmod +x /usr/local/bin/src
```

### Linux

```bash
curl -L https://sourcegraph.com/.api/src-cli/src_linux_amd64 -o /usr/local/bin/src
chmod +x /usr/local/bin/src
```

### Windows

**NOTE:** Windows support is still rough around the edges, but is available. If you encounter issues, please let us know by filing an issue :)

Run in PowerShell as administrator:

```powershell
New-Item -ItemType Directory 'C:\Program Files\Sourcegraph'
Invoke-WebRequest https://sourcegraph.com/.api/src-cli/src_windows_amd64.exe -OutFile 'C:\Program Files\Sourcegraph\src.exe'
[Environment]::SetEnvironmentVariable('Path', [Environment]::GetEnvironmentVariable('Path', [EnvironmentVariableTarget]::Machine) + ';C:\Program Files\Sourcegraph', [EnvironmentVariableTarget]::Machine)
$env:Path += ';C:\Program Files\Sourcegraph'
```

Or manually:

- Download the latest src_windows_amd64.exe: https://sourcegraph.com/.api/src-cli/src_windows_amd64.exe and rename to `src.exe`.
- Place the file under e.g. `C:\Program Files\Sourcegraph\src.exe`
- Add that directory to your system path to access it from any command prompt

### Renaming `src` (optional)

If you have a naming conflict with the `src` command, such as a Bash alias, you can rename the static binary. For example, on Linux / Mac OS:

```sh
mv /usr/local/bin/src /usr/local/bin/sourcegraph-cli
```

You can then invoke it via `sourcegraph-cli`.

## Usage

Consult `src -h` and `src api -h` for usage information.

## Authentication

Some Sourcegraph instances will be configured to require authentication. You can do so via the environment:

```sh
SRC_ENDPOINT=https://sourcegraph.example.com SRC_ACCESS_TOKEN="secret" src ...
```

Or via the configuration file (`~/src-config.json`):

```sh
	{"accessToken": "secret", "endpoint": "https://sourcegraph.example.com"}
```

See `src -h` for more information on specifying access tokens.

To acquire the access token, visit your Sourcegraph instance (or https://sourcegraph.com), click your username in the top right to open the user menu, select **Settings**, and then select **Access tokens** in the left hand menu.

## Development

If you want to develop the CLI, you can install it with `go get`:

```
go get -u github.com/sourcegraph/src-cli/cmd/src
```

## Releasing

1.  Find the latest version (either via the releases tab on GitHub or via git tags) to determine which version you are releasing.
2.  `VERSION=9.9.9 ./release.sh` (replace `9.9.9` with the version you are releasing)
3.  Travis will automatically perform the release. Once it has finished, **confirm that the curl commands fetch the latest version above**.
4.  Update the `MinimumVersion` constant in the [src-cli package](https://github.com/sourcegraph/sourcegraph/tree/master/internal/src-cli/consts.go).

### Patch releases

The version recommended by a Sourcegraph instance will be the highest patch version with the same major and minor version as the set minimum. This means that patch versions are reserved solely for non-breaking changes and minor bug fixes. This allows us to dynamically release fixes for older versions of src-cli without having to update the instance.

If you are releasing a bug fix or a new feature that is backwards compatible with one of the previous two minor version of Sourcegraph, the changes should be cherry-picked into a patch branch and re-releases with a new patch version. For example, suppose we have the the recommended versions.

| Sourcegraph version | Recommended src-cli version |
| ------------------- | --------------------------- | 
| `3.x`               | `3.y.z`                     |
| `3.{x-1}`           | `3.{y-1}.w`                 |

If a new feature is added to a new `3.y.{z+1}` release of src-cli and this change requires only features available in Sourcegraph `3.{x-1}`, then this feature should also be present in a new `3.{y-1}.{w+1}` release of src-cli. Similar logic should follow for Sourcegraph `3.{x-2}` and its recommended version of src-cli.

Because a Sourcegraph instance will automatically select the highest patch version, almost all non-breaking changes should increment only the patch version. The process above only necessary if a backwards-compatible change is made _after_ a backwards-incompatible one. If the recommended src-cli version for Sourcegraph `3.{x-1}` was instead `3.y.x` in the example above, there is no additional step required, and the new patch version of src-cli will be available to both Sourcegraph versions.
