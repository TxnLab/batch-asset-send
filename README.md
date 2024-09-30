# Batch Asset Sender

### Table of Contents

1. [Intro](#introduction)
2. [Installing from binaries](#installing-from-released-binaries)
2. [Building from source](#building-from-source)
2. [Command Line Arguments](#command-line-arguments)
3. [JSON Configuration](#json-configuration)
4. [Environment File](#environment-file)
5. [Results](#results)

## Introduction

This repository contains a simple command-line interface application written in Go which can _currently_ be used to:
* Distribute a set amount of a token (batch nft sends will come later) divided across NFD-based recipients, or a fixed amount per recipient.
* Provide various ways of selecting recipients, including:
    * All owned NFDs (not for sale)
    * All segments of a root
    * For the above, a random selection of X
    * For each match, send to the vault, or to the deposit account.
    * For deposit account sends, whether to make unique by owning account or NFD (ie: user own 5 NFDs w/ 1 account, do they get 5 distributions or 1).  This only applies to deposit account, not vaults, since vaults will always be unique per NFD.

## Installing from released binaries

Go to the latest released version at [releases](https://github.com/TxnLab/batch-asset-send/releases), and download the archive corresponding to your OS and architecture.
amd(intel)64-bit, or arm 64-bit.  The darwin (mac os) binaries aren't signed, so you'll have to right-click, pick open and pre-approve.  If you're not sure what this means, just build from source.
Extract the binaries to the directory of yuor choice and run as you would any other executable program.  See [Command Line Arguments](#command-line-arguments) 


## Building from source

First, ensure that Go is installed on your system. If it is not, you can download and install it from
the [official Go website](https://golang.org/dl/).

Next, you need Git installed.  You may installed it from the [official Git website](https://git-scm.com/downloads).  You can also just download an archive from github if desired.

The objective of this readme isn't to teach you Go, or Git but at least give you the basics to compile and run this code.

To ***clone***:

You need to figure out where you want this code to reside.  When you clone a git repository then it will by default create a subdirectory named after the git repo and place the contents there.

Run:
```shell
git clone https://github.com/TxnLab/batch-asset-send.git
cd batch-asset-send
```

To ***build*** the executable for your platform, simply run:

```shell
go build
```

This will create batch-asset-send (or batch-asset-send.exe) in the current directory.  This is the built program and can be copied elsewhere if you'd like.

## Command Line Arguments

The application accepts a series of command-line arguments. Each of these will be discussed below.

```
> ./batch-assent-send -h
Usage of ./batch-asset-send:
  -config string
    	path to json config file specifying what to send and to what recipients (default "send.json")
  -dryrun
    	dryrun just shows what would've been sent but doesn't actually send
  -network string
    	network: mainnet, testnet, betanet, or override w/ ALGO_XX env vars (default "mainnet")
  -parallel int
    	maximum number of sends to do at once - target node may limit (default 40)
  -sender string
    	account which has to sign all transactions - must have mnemonics in a [xx]_MNEMONIC[_xx] var
  -vault string
    	Don't send from sender account but from the named NFD vault that sender is owner of
```

The minimum parameters for use are the -sender parameter.
This specifies the public address of the account which will be **signing** the transactions.  If using the -vault {nfd name} argument, then it must be the owner of the NFD.  Most arguments have sensible defaults.

The sender MUST have mnemonics defined either as an xxxx_MNEMONIC environment variable or in a local .env file setting the same.

The parameters you specify for what to send MUST be specified in a json config file.
The default is to read from a send.json file in the current directory, but this can be overriden on the command line.

## JSON Configuration

The application also accepts a JSON configuration file as input. Here is an example configuration file with all possible options shown.  Any value left out is assumed 'false' or 0.
Some of these options conflict with eachother if specified together. 

```json
{
  "send": {
    "asset": {
      "asa": 123456,
      "amount": 1000000,
      "isPerRecip": false
    }
  },
  "destination": {
    "csvFile": "path to csv file",
    "segmentsOfRoot": "orange.algo",
    "allowDuplicateAccounts": true,
    "onlyRoots": false,
    "randomNFDs": {
      "count": 100
    },
    "verifiedRequirements": ["twitter", "caAlgo"],
    "sendToVaults": true
  }
}
```

**Send**: This will likely change significiantly in the future, but right now this just lists the single asset to send (a fungible token).
- `asa`: The id of the asset to send.
- `amount`: The amount of asset to send.  This is in the denominated units of the Asset, not its base units.  ie: Assume sending ALGO then 1.5 here really means 1,500,000 microAlgo.
- `isPerRecip`: Determines whether the amount is per recipient or the total amount to send.  If amount is 100 and isPerRecip is not set or false, then 100 is divided across all recipients.  If isPerRecip is set, then it would be 100 per recipient.

**Destination**: This configures the recipients of the assets.
- `csvFile`: Path to CSV file to load NFD names from (makes some options irrelevant). The first row must contain column names - with the column containing the nfd name named either nfd or name.π
- `segmentsOfRoot`: The root segments of the destination.
  - If specified, the NFDs are just those which are segments of a particular root NFD.  If not specified, then ALL nfds are the starting point. 
- `allowDuplicateAccounts`: Determines whether duplicate accounts are allowed (defaulting to no duplicates)
  - The owner of each NFD is used and if allowDuplicateAccounts is false, then only unique owners are chosen amongst the NFDs (picking an artbirary NFD for that owner)
- `onlyRoots`: Determines whether only root NFDs are allowed.
  - If specified, only roots are chosen with segments being skipped.
- `randomNFDs`: 
  - `count`: If specified, this is the number of NFDS to choose randomly from the total list.  ie: All segments of root X, but only pick 100 random recipients by specifying a count here.
- `verifiedRequirements`: An optional array of verified field names.  If specified, the destination NFD must have ALL of the specified verified fields.
  - The field names are case-sensitive.  All should be lowercase, but caAlgo is special and is the verified list of algorand addresses. 
- `sendToVaults`: Determines whether to send to vaults.
  - This is a key option and for most 'aidrops' should be chosen.  The recipient doesn't have to be opted-in before-hand.  As the sender you have to pay the .1 MBR fee per asset (only if their vault isn't already opted-in).

## Environment File

You may specify multiple options in an .env file, or in the local environment.

Some of the supported properties are:

* ALGORAND_DATA
  * If specified, tries to load node configuration data from the algod.net / algod.token file in this dir.
* ALGO_NFD_URL
  * The https:// address of the NFD API (defaulted for you - for each network)
* ALGO_ALGOD_URL / ALGO_ALGOD_TOKEN
  * URL to algod endpoint and token (if needed) - defaults to algonode
* ALGO_ALGOD_HEADERS
  * Rarely needed - but allows header:value,header:value pairs - adds to headers passed to algod node requests.

## Results

It's simple but currently the results of each send are appended to success.txt and failure.txt files in the current directory.
The failure count will be reported at the end.  If any fail, you should check the failures reported and possibly send manually. 

---
### Note on use of NFD Api

The NFD API client was generated via the interactive swagger link from https://api-docs.nf.domains using Generate
Client->Go.
The downloaded library was inserted into the lib/nfdapi/swagger directory with minimal modifications (mostly to change the package name)

One caveat is that at least for the Go generated code, some generated types used int32 when they should have been uint64.
The asa/app id fields for example.  A few of the structs (but not all) were modified to change to uint64 types.

