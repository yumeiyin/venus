# Go-filecoin code overview

This document provides a high level tour of the go-filecoin implementation of the Filecoin protocols in Go.

This document assumes a reasonable level of knowledge about the Filecoin system and protocols, which are not re-explained here. 
It is complemented by specs (link forthcoming) that describe the key concepts implemented here.

**Table of contents**
<!-- 
    TOC generated by https://github.com/thlorenz/doctoc
    Install with `npm install -g doctoc`.
    Regenerate with `doctoc CODEWALK.md`.
    It's ok to edit manually if you don't have/want doctoc.
 -->
<!-- START doctoc generated TOC please keep comment here to allow auto update -->
<!-- DON'T EDIT THIS SECTION, INSTEAD RE-RUN doctoc TO UPDATE -->


- [Background](#background)
- [Architecture overview](#architecture-overview)
- [A tour of the code](#a-tour-of-the-code)
  - [History–the Node object](#historythe-node-object)
  - [Core services](#core-services)
  - [Plumbing & porcelain](#plumbing--porcelain)
  - [Commands](#commands)
  - [Protocols](#protocols)
  - [Network layer](#network-layer)
  - [Entry point](#entry-point)
  - [Testing](#testing)
- [Filesystem storage](#filesystem-storage)
  - [JSON Config](#json-config)
  - [Datastores](#datastores)
  - [Keystore](#keystore)
- [Dependencies](#dependencies)
- [Patterns](#patterns)
  - [Plumbing and porcelain](#plumbing-and-porcelain)
  - [Consumer-defined interfaces](#consumer-defined-interfaces)

<!-- END doctoc generated TOC please keep comment here to allow auto update -->

## Background

The go-filecoin implementations is the result of combined research and development effort.
The protocol spec and architecture evolved from a prototype, and is the result of iterating towards our goals. 
Go-filecoin is a work in progress.
We are still working on clarifying the architecture and propagating good patterns throughout the code.
Please bear with us, and we’d love your help.

Filecoin borrows a lot from the [IPFS](https://ipfs.io/) project, including some patterns, tooling, and packages. 
Some benefits of this include:

- the projects encode data in the same way ([IPLD](https://ipld.io/), 
[CIDs](https://github.com/multiformats/cid)), easing interoperability;
- the go-filecoin project can build on solid libraries like the IPFS commands.

Other patterns, we've evolving for our needs:

- go-ipfs relies heavily on shell-based integration testing; we aim to rely heavily on unit testing and Go-based integration tests.
- The go-ipfs package structure involves a deep hierarchy of dependent implementations; 
we're moving towards a more Go-idiomatic approach with narrow interfaces defined in consuming packages (see [Patterns](#patterns).
- The term “block” is heavily overloaded: a blockchain block ([`types/block.go`](https://github.com/filecoin-project/go-filecoin/tree/master/types/block.go)), 
but also content-id-addressed blocks in the block service. 
Blockchain blocks are stored in block service blocks, but are not the same thing.

## Architecture overview

```
           ┌─────────────────────────────────────┐
           │                                     │
  Network  │  network (gossipsub, bitswap, etc.) │                 | | \/
           │                                     │                 |_| /\
           └─────▲────────────▲────────────▲─────┘
                 │            │            │           ┌────────────────────────────┐
           ┌─────▼────┐ ┌─────▼─────┐ ┌────▼─────┐     │                            │
           │          │ │           │ │          │     │    Commands / REST API     │
Protocols  │ Storage  │ │  Mining   │ │Retrieval │     │                            │
           │ Protocol │ │ Protocol  │ │ Protocol │     └────────────────────────────┘
           │          │ │           │ │          │                    │
           └──────────┘ └───────────┘ └──────────┘                    │
                 │            │             │                         │
                 └──────────┬─┴─────────────┴───────────┐             │
                            ▼                           ▼             ▼
           ┌────────────────────────────────┐ ┌───────────────────┬─────────────────┐
 Internal  │            Core API            │ │     Porcelain     │     Plumbing    │
      API  │                                │ ├───────────────────┘                 │
           └────────────────────────────────┘ └─────────────────────────────────────┘
                            │                                    │
                  ┌─────────┴────┬──────────────┬──────────────┬─┴────────────┐
                  ▼              ▼              ▼              ▼              ▼
           ┌────────────┐ ┌────────────┐ ┌────────────┐ ┌────────────┐ ┌────────────┐
           │            │ │            │ │            │ │            │ │            │
     Core  │  Message   │ │   Chain    │ │ Processor  │ │   Block    │ │   Wallet   │
           │    Pool    │ │   Store    │ │            │ │  Service   │ │            │
           │            │ │            │ │            │ │            │ │            │
           └────────────┘ └────────────┘ └────────────┘ └────────────┘ └────────────┘
```
## A tour of the code

### History–the Node object

The `Node` ([`node/`](https://github.com/filecoin-project/go-filecoin/tree/master/node)) object is the “server”. 
It contains much of the core protocol implementation and plumbing. 
As an accident of history it has become something of a god-object, which we are working to resolve. 
The `Node` object is difficult to unit test due to its many dependencies and complicated set-up. 
We are [moving away from this pattern](https://github.com/filecoin-project/go-filecoin/issues/1469#issuecomment-451619821),
and expect the Node object to be reduced to a glorified constructor over time.

The [`api`](https://github.com/filecoin-project/go-filecoin/tree/master/api) package contains the API of all the 
core building blocks upon which the protocols are implemented. 
The implementation of this API is the `Node`. 
We are migrating away from this `api` package to the plumbing package, see below.

The [`protocol`](https://github.com/filecoin-project/go-filecoin/tree/master/protocol) package contains much of the application-level protocol code. 
The protocols are implemented in terms of the Node API (old) as well as the new plumbing & porcelain APIs (see below).
Currently the hello, retrieval and storage protocols are implemented here. 
Block mining should move here (from the [`mining`](https://github.com/filecoin-project/go-filecoin/tree/master/mining) top-level package and `Node` internals). 
Chain syncing may move here too.

### Core services

At the bottom of the architecture diagram are core services. 
These are focused implementations of some functionality that don’t achieve much on their own, but are the means to the end. 
Core services are the bottom level building blocks out of which application functionality can be built.

They are the source of truth in all data.

Core services are mostly found in top-level packages. 
Most are reasonably well factored and testable in isolation.

Services include (not exhaustive):

- Message pool: hold messages that haven’t been mined into a block yet.
- Chain store: stores & indexes blockchain blocks.
- Chain syncer: syncs chain blocks from the rest of the network.
- Processor: Defines how transaction messages drive state transitions.
- Block service: content-addressed key value store that stores IPLD data, including blockchain blocks as well as the state tree (it’s poorly named).
- Wallet: manages keys.

### Plumbing & porcelain

The [`plumbing`](https://github.com/filecoin-project/go-filecoin/tree/master/plumbing) & 
[`porcelain`](https://github.com/filecoin-project/go-filecoin/tree/master/porcelain) packages are 
the new API; 
over time, this patterns should completely replace the existing top-level `api` package and its implementation in `Node`. 
The plumbing & porcelain design pattern is explained in more detail below.

Plumbing is the set of public apis required to implement all user-, tool-, and protocol-level features. 
Plumbing implementations depend on the core services they need, but not on the `Node`.
Plumbing is intended to be fairly thin, routing requests and data between core components. 
Plumbing implementations are often tested with real implementations of the core services they use, but can also be tested with fakes and mocks.

Porcelain implementations are convenience compositions of plumbing. 
They depend only on the plumbing API, and can coordinate a sequence of actions. 
Porcelain is ephemeral; the lifecycle is the duration of a single porcelain call: something calls into it, it does its thing, and then returns. 
Porcelain implementations are ideally tested with fakes of the plumbing they use, but can also use full implementations. 

### Commands

The `go-filecoin` binary can run in two different modes, either as a long-running daemon exposing a JSON/HTTP RPC API, 
or as a command-line interface which interprets and routes requests as RPCs to a daemon. 
In typical usage, you start the daemon with `go-filecoin daemon` then use the same binary to issue commands like `go-filecoin wallet addrs`, 
which are transmitted to the daemon over the HTTP API.

The commands package uses the [go-ipfs command library](https://github.com/ipfs/go-ipfs-cmds) and defines commands as both CLI and JSON entry points.

[Commands](https://github.com/filecoin-project/go-filecoin/tree/master/commands) implement user- and tool-facing functionality. 
Command implementations should be very, very small. 
With no logic of their own, they should call just into a single plumbing or porcelain method (never into core APIs directly). 
The go-ipfs command library introduces some boilerplate which we can reduce with some effort in the future. 
Right now, some of the command implementations call into the node; this should change.

Tests for commands are generally end-to-end “daemon tests” that exercise CLI. 
They start some nodes and interact with them through shell commands. 

### Protocols

[Protocols](https://github.com/filecoin-project/go-filecoin/tree/master/protocol) embody 
“application-level” functionality. They are persistent; they keep running without active user/tool activity. 
Protocols interact with the network. 
Protocols depend on `plumbing` and `porcelain` for their implementation, as well some "private" core APIs (at present, many still depend on the `Node` object).

Protocols drive changes in, but do not own, core state. 
For example, the chain sync protocol drives updates to the chain store (a core service), but the sync protocol does not own the chain data.
However, protocols may maintain their own non-core, protocol-specific datastores (e.g. unconfirmed deals). 

Application-level protocol implementations include:

- Storage protocol: the mechanism by which clients make deals with miners, transfer data for storage, and then miners prove storage.
- Block mining protocol: the protocol for block mining and consensus. 
Miners who are storing data participate in creating new blocks. 
Miners win elections in proportion to storage committed. 
This block mining is spread through a few places in the code. 
Much in mining package, but also a bunch in the node implementation.
- Chain protocol: protocol for exchange of mined blocks

More detail on the individual protocols is coming soon.

### Actors

Actors are Filecoin’s notion of smart contracts. 
They are not true smart contracts—with bytecode running on a VM—but instead implemented in Go. 
It is expected that other implementations will match the behaviour of the Go actors exactly. 
An ABI describes how inputs and outputs to the VM are encoded. 
Future work will replace this implementation with a “real” VM.

The [Actor](https://github.com/filecoin-project/go-filecoin/blob/master/actor/actor.go) struct is the base implementation of actors, with fields common to all of them.

- `Code` is a CID identifying the actor code, but since these actors are implemented in Go, is actually some fixed bytes acting as an identifier. 
This identifier selects the kind of actor implementation when a message is sent to its address.
- `Head` is the CID  of this actor instance’s state.
- `Nonce` is a counter of #messages received *from* that actor. 
It is only set for account actors (the actors from which external parties send messages); messages between actors in the VM don’t use the nonce.
- `Balance` is FIL balance the actor controls. 

Some actors are singletons (e.g. the storage market) but others have multiple instances (e.g. storage miners). 
A storage miner actor exists for each miner in the Filesystem network. 
Their structs share the same code CID so they have the same behavior, but have distinct head state CIDs and balance. 
Each actor instance exists at an address in the state tree. An address is the hash of the actor’s public key.

The [account](https://github.com/filecoin-project/go-filecoin/blob/master/actor/builtin/account) actor doesn’t have any special behavior or state other than a balance. 
Everyone who wants to send messages (transactions) has an account actor, and it is from this actor’s address that they send messages.

Every storage miner has an instance of a [miner](https://github.com/filecoin-project/go-filecoin/blob/master/actor/builtin/miner) actor. 
The miner actor plays a role in the storage protocol, for example it pledges space and collateral for storage, posts proofs of storage, etc. 
A miner actor’s state is located in the state tree at its address; the value found there is an Actor structure. 
The head CID in the actor structure points to that miner’s state instance (encoded).

Other built-in actors include the [payment broker](https://github.com/filecoin-project/go-filecoin/blob/master/actor/builtin/paymentbroken), 
which provides a mechanism for off-chain payments via payment channels, 
and the [storage market](https://github.com/filecoin-project/go-filecoin/blob/master/actor/storagemarket), 
which starts miners and tracks total storage (aka “power”). 
These are both singletons.

Actors declare a list of exported methods with ABI types. 
Method implementations typically load the state tree, perform some query or mutation, then return a value or an error. 

### The state tree

Blockchain state is represented in the [state tree](https://github.com/filecoin-project/go-filecoin/blob/master/state/tree.go), 
which contains the state of all actors. 
The state tree is a map of address to (encoded) actor structs. 
The state tree interface exposes getting and setting actors at addresses, and iterating actors. 
The underlying data structure is a [Hash array-mapped trie](https://en.wikipedia.org/wiki/Hash_array_mapped_trie). 
A HAMT is also often used to store actor state, eg when the actor wants to store a large map.

The canonical binary encoding used by Filecoin is [CBOR](http://cbor.io/). In Go, structs are CBOR-encoded by reflection. 
The ABI uses a separate inner encoding, which is manual. 

### Messages and state transitions

Filecoin state transitions are driven by messages sent to actors; these are our “transactions”. 
A message is a method invocation on an actor. 
A message has sender and recipient addresses, and optional parameters such as an amount of filecoin to transfer, a method name, and parameters.

Messages from the same actor go on chain in nonce order. 
Note that the nonce is only really used by account actors (representing external entities such as humans). 
The nonce guards against replay of messages entering the VM for execution, but is not used for messages between actors during an external message’s execution. 

Driving a state transition means invoking an actor method. 
One invokes a method on an actor by sending it a message. 
To send a message the message is created, signed, added to your local node’s message pool broadcast on the network to other nodes, 
which will add it to their message pool too. 
Some node will then mine a block and possibly include your message. 
In Filecoin, it is essential to remember that sending the message does not mean it has gone on chain or that its outcome has been reflected in the state tree. 
Sending means the message is available to be mined into a block. 
You must wait for the message to be included in a block to see its effect.

Read-only methods, or query messages, are the mechanism by which actor state can be inspected. 
These messages are executed locally against a read only version of the state tree of the head of the chain. 
They never leave the node, they are not broadcast. 
The plumbing API exposes `MessageSend` and `MessageQuery` for these two cases. 

The [processor](https://github.com/filecoin-project/go-filecoin/blob/master/consensus/processor.go) is the 
entry point for making and validating state transitions represented by the messages. 
It is modelled Ethereum’s message processing system. 
The processor manages the application of messages to the state tree from the prior block/s. 
It loads the actor from which a message came, check signatures, 
then loads the actor and state to which a message is addressed and passes the message to the VM for execution. 

The [vm](https://github.com/filecoin-project/go-filecoin/blob/master/vm) package has the low level detail of calling actor methods. 
A [VM context](https://github.com/filecoin-project/go-filecoin/blob/master/vm/context.go) defines the world visible from an actor implementation while executing, 

### Consensus

Filecoin uses a consensus algorithm called [expected consensus](https://github.com/filecoin-project/go-filecoin/blob/master/consensus/expected.go). 
Unlike proof-of-work schemes, expected-consensus is a proof-of-stake model, where probability of mining a block in each round (30 seconds) 
is proportional to amount of storage a miner has committed to the network. 
Each round, miners are elected through a probabilistic but private mechanism akin to rolling independent, private, but verifiable dice. 
The expected number of winners in each round is one, but it could be zero or more than one miner. 
If a miner is elected, they have the right to mine a block in that round.

Given the probabilistic nature of mining new blocks, more than one block may be mined in any given round. 
Hence, a new block might have more than one parent block. 
The parents form a set, which we call a [tipset](https://github.com/filecoin-project/go-filecoin/blob/master/consensus/tipset.go). 
All the blocks in a tipset are at the same height and share the same parents. 
Tipsets contain one or more blocks. 
A null block count indicates the absence of any blocks mined in a previous round. 
Subsequent blocks are built upon *all* of the tipset; 
there is a canonical ordering of the messages in a tipset defining a new consensus state, not directly referenced from any of the tipset’s blocks.

### Network layer

Filecoin relies on [libp2p](https://libp2p.io/) for all its networking, such as peer discovery, NAT discovery, and circuit relay. 
Filecoin uses two transport protocols from libp2p:

- [GossipSub](https://github.com/libp2p/specs/tree/master/pubsub/gossipsub) for pubsub gossip among peers propagating blockchain blocks and unmined messages.
- [Bitswap](https://github.com/ipfs/specs/tree/master/bitswap) for exchanging binary data.

### Entry point

There’s no centrally dispatched event loop. 
The node starts up all the components, connects them as needed, and waits. 
Protocols (goroutines) communicate through custom channels. 
This architecture needs more thought, but we are considering moving more inter-module communication to use iterators (c.f. those in Java). 
An event bus might also be a good pattern for some cases, though.

## Filesystem storage

The *repo*, aka `fsrepo`, is a directory stored on disk containing all necessary information to run a `go-filecoin daemon`, typically at `$HOME/.filecoin`. 
The repo does not include client data stored by storage miners, which is held instead in the sector base. 
The repo does include a JSON config file with preferences on how the daemon should operate, 
several key value datastores holding data important to the internal services, 
and the keystore which holds private key data for encryption.

### JSON Config

The JSON config file is stored at `$HOME/.filecoin/config.json`, and can be easily edited using the `go-filecoin config` command. 
Users can also edit the file directly at their own peril.

### Datastores

The key-value datastores in the repo include persisted data from a variety of systems within Filecoin. 
Most of them hold CBOR encoded data keyed on CID, however this varies. 
The key value stores include the badger, chain, deals, and wallet directories under `$HOME/.filecoin`.

The purpose of these directories is:
- _Badger_ is a general purpose datastore currently only holding the genesis key, but in the future, 
almost all our datastores should be merged into this one.
- _Chain_ is where the local copy of the blockchain is stored.
- _Deals_ is where the miner and client store persisted information on open deals for data storage, 
essentially who is storing what data, for what fee and which sectors have been sealed.
- _Wallet_ is where the user’s Filecoin wallet information is stored.

### Keystore

The keystore contains the binary encoded peer key for interacting securely over the network. 
This data lives in a file at `$HOME/.filecoin/keystore/self`.

## Testing

The `go-filecoin` codebase has a few different testing mechanisms: 
unit tests, in-process integration tests, “daemon” integration tests, and a couple of high level functional tests.

Many parts of code have good unit tests. 
We’d like all parts to have unit tests, but in some places it hasn’t been possible where prototype code omitted testability features. 
Functionality on the `Node` object is a prime example, which we are [moving away from](#plumbing-and-porcelain). 

Historically there has been a prevalence of integration-type testing. 
Relying only on integration tests can make it hard to verify small changes to internals. 
We’re driving towards both wide unit test coverage, with integration tests to verifying end-to-end behaviour.

There are two patterns for unit tests. 
In plumbing and low level components, many tests use real dependencies (or at least in-memory versions of them). 
For higher level components like porcelain or protocols, where dependencies are more complex to set up, 
we often use fake implementations of just those parts of the plumbing that are required. 
It is a goal to have both unit tests (with fakes or real deps), as well as higher level integration-style tests. 

Code generally uses simple manual dependency injection. 
A component that takes a large number of deps at construction can have them factored into a struct.
A module should often (re-)declare a narrow subset of the interfaces it depends on (see [Consumer-defined interfaces](#consumer-defined-interfaces))), in canonical Go style. 

Some [node integration tests](https://github.com/filecoin-project/go-filecoin/blob/master/node/node_test.go) start one or more full nodes in-process. 
This is useful for fine-grained control over the node being tested. 
Setup for these tests is a bit difficult and we aim to make it much easier to instantiate and test full nodes in-process.

Daemon tests are end-to-end integration tests that exercise the command interface of the `go-filecoin` binary. 
These execute separate `go-filecoin` processes and drive them via the command line. 
These tests are mostly under the [`commands`](https://github.com/filecoin-project/go-filecoin/blob/master/commands) package, 
and use [TestDaemon](https://github.com/filecoin-project/go-filecoin/blob/master/testhelpers/commands.go). 
Because the test and the daemon being tested are in separate processes, getting access to the daemon process’s output streams or attaching a debugger is tricky; 
see comments in [createNewProcess][(https://github.com/filecoin-project/go-filecoin/blob/726e6705860ddfc8ca4e55bc3610ad2230a95c0c/testhelpers/commands.go#L849)

In daemon tests it is important to remember that messages do not have any effect on chain state until they are mined into a block. 
Preparing an actor in order to receive messages and mutate state requires some delicate network set-up, mining messages into a block to create the actor before it can receive messages. 
See `MineOnce` in [`mining/scheduler`](https://github.com/filecoin-project/go-filecoin/blob/master/mining/scheduler.go) which synchronously performs a round of block mining and then stops, pushing the test state forward.

The `functional-tests` directory contains some Go and Bash scripts which perform complicated multi-node tests on our continuous build. 
These are not daemon tests, but run separately.

Some packages have a `testing.go` file with helpers for setting up tests involving that package’s types. 
The [`types/testing.go`](https://github.com/filecoin-project/go-filecoin/blob/master/types/testing.go) file has some more generally useful constructors. 
There is also a top-level [`testhelpers`](https://github.com/filecoin-project/go-filecoin/blob/master/testhelpers) package with higher level helpers, often used by daemon tests.

We’re in process of creating the Filecoin Automation and Systems Toolkit (FAST) [library](https://github.com/filecoin-project/go-filecoin/tree/master/tools/fast). 
The goal of this is to unify duplicated code paths which bootstrap and drive `go-filecoin` daemons for daemon tests, functional tests, 
and network deployment verification, providing a common library for filecoin automation in Go. 

Tests are typically run with `go run ./build/*.go test`. 
It passes flags to `go test` under the hood, so you can provide `-run <regex>` to run a subset of tests. 
Vanilla `go test` also works, after build scripts have built and installed appropriate dependencies.

## Dependencies

Dependencies in go-filecoin are managed by [gx](https://github.com/whyrusleeping/gx), a content-addressed dependency manager. 
You’ll notice that the hash of a dependency’s content appears in the import path. 
Almost all runtime dependencies are managed by gx (mostly being other Protocol Labs-sponsored projects). 

The `gx-go` manages a package.json file. 
In order to be imported by gx, a package needs to be “gxed”. 
See the [gx-go repo](https://github.com/whyrusleeping/gx-go) for details about preparing a package for gxing and importing it into the project. 
If you want to depend on a package whos author has not gxed it, we can fork it and gx our fork.

gx came about before Go modules, which aim to solve many of the same problems. 
The IPFS project and go-filecoin [may migrate to Go modules](https://github.com/ipfs/go-ipfs/issues/5850) in the future.

## Patterns

The project makes heavy use of or is moving towards a few key design patterns, explained here.

### Plumbing and porcelain

The _plumbing and porcelain_ pattern is [borrowed from Git](https://git-scm.com/book/en/v2/Git-Internals-Plumbing-and-Porcelain).
Plumbing and porcelain form the API to the internal [core services](#core-services), and will replace the `api` package.

*Plumbing* is the small set of public building blocks of queries and operations that protocols, clients and humans want to use with a Filecoin node. 
These are things like `MessageSend`, `GetHead`, `GetBlockTime`, etc. 
By fundamental, we mean that it doesn't make sense to expose anything lower level. 
The bar for adding new plumbing is high. 
It is very important, for testing and sanity, that plumbing methods be implemented in terms of their narrowest actual dependencies on core services,
and that they not depend on Node or another god object.

The plumbing API is defined by its implementation in [plumbing/api.go](https://github.com/filecoin-project/go-filecoin/blob/master/plumbing/api.go).
Consumers of plumbing (re-)define the subset of plumbing on which they depend, which is an idiomatic Go pattern (see below).
Implementations of plumbing live in their own concisely named packages under [plumbing](https://github.com/filecoin-project/go-filecoin/tree/master/plumbing).

*Porcelain* are calls on top of the plumbing API.
A porcelain call is a useful composition of plumbing calls and is implemented in terms of calls to plumbing. 
An example of a porcelain call is `CreateMiner == MessageSend + MessageWait + ConfigUpdate`. 
The bar is low for creation of porcelain calls. 
Porcelain calls should define the subset of the plumbing interface on which they depend for ease of testing.

Porcelain lives in a single [porcelain](https://github.com/filecoin-project/go-filecoin/blob/master/porcelain/) package. 
Porcelain calls are free functions that take the plumbing interface as an argument. 
The call defines the subset of the plumbing interface that it needs, which can be easily faked in testing.

We are in the [process of refactoring](https://github.com/filecoin-project/go-filecoin/issues/1469) all protocols to depend only on porcelain, 
plumbing and other core APIs, instead of on the Node.

### Consumer-defined interfaces

Go interfaces generally belong in the package that *uses* values of the interface type, not the package that implements those values. 
This embraces [Postel's law](https://en.wikipedia.org/wiki/Robustness_principle), 
reducing direct dependencies between packages and making them easier to test. 
It isolates small changes to small parts of the code.

Note that this is quite different to the more common pattern in object-oriented languages, where interfaces are defined near their implementations.
Our implementation of [plumbing and porcelain](#plumbing-and-porcelain) embraces this pattern, and we are adopting it more broadly.

This idiom is unfortunately hidden away in a [wiki page about code review](https://github.com/golang/go/wiki/CodeReviewComments#interfaces).
See also Dave Cheney on [SOLID Go Design](https://dave.cheney.net/2016/08/20/solid-go-design).