# Sourcegraph CLI [![Build Status](https://travis-ci.org/sourcegraph/src-cli.svg)](https://travis-ci.org/sourcegraph/src-cli) [![Go Report Card](https://goreportcard.com/badge/sourcegraph/src-cli)](https://goreportcard.com/report/sourcegraph/src-cli)

<img src="https://user-images.githubusercontent.com/3173176/43567326-3db5f31c-95e6-11e8-9e74-4c04079c01b0.png" width=450 align=right>

**Quick links**:
- [Installation](#installation)
- [Setup](#setup) ([Authentication](#authentication))
- [Usage](#usage)

The Sourcegraph `src` CLI provides access to [Sourcegraph](https://sourcegraph.com) via a command-line interface.

It currently provides the ability to:

- **Execute search queries** from the command line and get nice colorized output back (or JSON, optionally).
- **Execute GraphQL queries** against a Sourcegraph instance, and get JSON results back (`src api`).
  - You can provide your API access token via an environment variable or file on disk.
  - You can easily convert a `src api` command into a curl command with `src api -get-curl`.
- **Manage repositories, users, and organizations** using the `src repos`, `src users`, and `src orgs` commands.
- **Execute campaign actions** as part of [Sourcegraph campaigns](https://docs.sourcegraph.com/user/campaigns)

If there is something you'd like to see Sourcegraph be able to do from the CLI, let us know! :)

## Installation

For Sourcegraph 3.13 and newer, the preferred method of installation is to ask _your_ Sourcegraph instance for the latest compatible version. To do this, replace `https://sourcegraph.com` in the commands below with the address of your instance.

For Sourcegraph 3.12 and older, run the following commands verbatim (against sourcegraph.com) or install from one of the published [releases on GitHub](https://github.com/sourcegraph/src-cli/releases).

```
https://github.com/sourcegraph/src-cli/releases/download/{version}/{binary}
```

#### Requirements

If you want to use the `src action exec` functionality (see [Sourcegraph campaigns](https://docs.sourcegraph.com/user/campaigns) docs and `src action exec -h`), make sure that `git` is installed and accessible by `src`.

#### Mac OS

```bash
# Sourcraph 3.13 and newer:
curl -L https://<your-sourcegraph-instance>/.api/src-cli/src_darwin_amd64 -o /usr/local/bin/src
chmod +x /usr/local/bin/src

# Sourcraph 3.12 and older:
curl -L https://sourcegraph.com/.api/src-cli/src_darwin_amd64 -o /usr/local/bin/src
chmod +x /usr/local/bin/src
```

or use `brew` to get the newest version:

```
brew install sourcegraph/src-cli/src-cli
```

#### Linux

```bash
# Sourcraph 3.13 and newer:
curl -L https://<your-sourcegraph-instance>/.api/src-cli/src_linux_amd64 -o /usr/local/bin/src
chmod +x /usr/local/bin/src

# Sourcraph 3.12 and older:
curl -L https://sourcegraph.com/.api/src-cli/src_linux_amd64 -o /usr/local/bin/src
chmod +x /usr/local/bin/src
```

#### Windows

**NOTE:** Windows support is still rough around the edges, but is available. If you encounter issues, please let us know by filing an issue :)

Run in PowerShell as administrator:

```powershell
New-Item -ItemType Directory 'C:\Program Files\Sourcegraph'

# Sourcegraph 3.13 and newer:
Invoke-WebRequest https://<your-sourcegraph-instance>/.api/src-cli/src_windows_amd64.exe -OutFile 'C:\Program Files\Sourcegraph\src.exe'
# Sourcegraph 3.12 and older:
Invoke-WebRequest https://sourcegraph.com/.api/src-cli/src_windows_amd64.exe -OutFile 'C:\Program Files\Sourcegraph\src.exe'

[Environment]::SetEnvironmentVariable('Path', [Environment]::GetEnvironmentVariable('Path', [EnvironmentVariableTarget]::Machine) + ';C:\Program Files\Sourcegraph', [EnvironmentVariableTarget]::Machine)
$env:Path += ';C:\Program Files\Sourcegraph'
```

Or manually:

- Download the latest src_windows_amd64.exe:
  - Sourcegraph 3.13 and newer: https://<your-sourcegraph-instance>/.api/src-cli/src_windows_amd64.exe
  - Sourcegraph 3.12 and older: https://sourcegraph.com/.api/src-cli/src_windows_amd64.exe
- Place the file under e.g. `C:\Program Files\Sourcegraph\src.exe`
- Add that directory to your system path to access it from any command prompt

#### Renaming `src` (optional)

If you have a naming conflict with the `src` command, such as a Bash alias, you can rename the static binary. For example, on Linux / Mac OS:

```sh
mv /usr/local/bin/src /usr/local/bin/sourcegraph-cli
```

You can then invoke it via `sourcegraph-cli`.

## Setup

To use `src` with your own Sourcegraph instance set the `SRC_ENDPOINT` environment variable:

```sh
SRC_ENDPOINT=https://sourcegraph.example.com src search
```

Or via the configuration file (`~/src-config.json`):

```sh
{"endpoint": "https://sourcegraph.example.com"}
```

### Authentication

Some Sourcegraph instances will be configured to require authentication. You can do so via the environment variable `SRC_ACCESS_TOKEN`:

```sh
SRC_ENDPOINT=https://sourcegraph.example.com SRC_ACCESS_TOKEN="secret" src ...
```

Or via the configuration file (`~/src-config.json`):

```sh
{"accessToken": "secret", "endpoint": "https://sourcegraph.example.com"}
```

See `src -h` for more information on specifying access tokens.

To acquire the access token, visit your Sourcegraph instance (or https://sourcegraph.com), click your username in the top right to open the user menu, select **Settings**, and then select **Access tokens** in the left hand menu.

## Usage

`src` provides different subcommands to interact with different parts of Sourcegraph:

 - `search` - search for results on the configured Sourcegraph instance
 - `api` - interacts with the Sourcegraph GraphQL API
 - `repos` (alias: `repo`) - manages repositories
 - `users` (alias: `user`) - manages users
 - `orgs` (alias: `org`) - manages organizations
 - `config` - manages global, org, and user settings
 - `extsvc` - manages external services
 - `extensions` (alias: `ext`) - manages extensions
 - `actions` - runs [campaign actions](https://docs.sourcegraph.com/user/campaigns/actions)to generate patch sets
 - `campaigns` - manages [campaigns](https://docs.sourcegraph.com/user/campaigns)
 - `lsif` - manages LSIF data
 - `version` - display and compare the src-cli version against the recommended version of the configured Sourcegraph instance

Run `src -h` and `src <subcommand> -h` for more detailed usage information.
