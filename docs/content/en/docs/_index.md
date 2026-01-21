---
title: About Porch
linkTitle: Docs
menu: {main: {weight: 20}}
weight: 1
---


<div style="border: 1px solid red; background-color: #ffe6e6; color: #b30000; padding: 1em; margin-bottom: 1em;">
  <strong>⚠️ Outdated Notice:</strong> The most up to date version of this restructure guide can be found
  <a href="https://lf-nephio.atlassian.net/wiki/spaces/HOME/pages/661749794/Porch+Documentation+Refactoring" target="_blank" rel="noopener noreferrer">here</a>.
</div>

The Kubernetes documentation follows a table of contents detailed in the below section. taking this template and adapting it to the porch code base/documentation we get the following.

1. The different sections required to be covered following the Kubernetes documentation.
2. The currently available documentation relating to this section available now on <https://docs.nephio.org/docs/porch/>.
3. The gaps/missing sections not found in the current documentation but required in the rework.

Sections marked as <span style="color: green;">[Section in Green]</span> means they have been reviewed and marked as necessary to be included in the new documentation.

Sections marked as <span style="color: red;">[Section in Red]</span> means they have not been reviewed but could still be placed in the docs in the given location once they are. TLDR its a topic of interest to be looked at just not signed off on yet as as a mandatory addition.

---

## Table of Contents

1. Overview
2. Concepts
3. Getting Started
4. Tutorials & How‑tos
5. Architecture & Components
6. Configuration & Deployment
7. CLI / API / Reference
8. Best Practices & Patterns
9. Troubleshooting & FAQ
10. Security & Compliance
11. Glossary
12. Contributing
13. Release Notes / Changelog

---

## 1. Overview

**Section Must Contain:**

* What is **Porch**
    • Short description (“Porch = package orchestration, opinionated package management …”) (Porch already has this under Overview). ([https://docs.nephio.org/docs/porch/](https://docs.nephio.org/docs/porch/))
* Goals & scope (what Porch intends to do, what it does *not* do)
* Audiences: which users should care (operators, GitOps engineers, developers, integrators, etc.)
* Prerequisites / compatibility (Kubernetes versions, environments, permissions, dependencies)

**Currently Available Resources:**

* **Porch documentation** ([https://docs.nephio.org/docs/porch/](https://docs.nephio.org/docs/porch/))
* *Overview* — what Porch is. (<https://docs.nephio.org/docs/porch/>) exists but requires refresh
* *Porch in the Nephio architecture, history and outlook* (exists but we should do away with it)

**Gaps / additions Required:**

* Statement of *goals & scope* (what Porch intends to do / NOT do).
* Target audiences (operators, developers, integrators).
* Supported environments / prerequisites summary.

<details>
<summary>More Detail on concepts requiring explanation</summary>

* Stuff here

</details>

---

## 2. Concepts

**Section Must Contain:**

* Key terminology (package, variant, mutation pipeline, function runner, etc.)
* Core models/entities: what Porch works with (packages, modules, variants, pipelines).
* High‑level flow / lifecycle: how a package moves through Porch (creation → mutation → deployment or consumption)
* Relationships to other Nephio components / external systems (Git repos, registries, etc.)

**Currently Available Resources:**

* [Porch Concepts](https://docs.nephio.org/docs/porch/package-orchestration/)
* [Configuration as Data](https://docs.nephio.org/docs/porch/config-as-data/)
* [Package Mutation Pipeline Order](https://docs.nephio.org/docs/porch/package-mutation-pipeline-order/)
* [Function Runner Pod Templating](https://docs.nephio.org/docs/porch/function-runner-pod-templates/)
* [Package Variant Controller](https://docs.nephio.org/docs/porch/package-variant/)

**Gaps / additions Required:**

* Central glossary of key terms
* Visual lifecycle diagram of a package
* Mapping of Porch functions vs Nephio components

<details>
<summary>More Detail on concepts requiring explanation</summary>

* <span style="color: green;">[PACKAGE-ORCHESTRATION]</span> <https://docs.nephio.org/docs/porch/package-orchestration/>  ← A LOT OF GREAT REUSABLE CONTENT HERE FOR THIS SECTION. THIS SECTION CAN BE OUR HIGH LEVEL INTRODUCTION OF THE ENTIRE PROJECT BEFORE DIVING DEEPER INTO THE DETAILS IN THE LATER SECTIONS BELOW.
* <span style="color: green;">[MODIFYING-PORCH-PACKAGES]</span> MUTATORS VS VALIDATORS
* <span style="color: green;">[REVISION/VERSION]</span> Explain what the revision of a package revision is.
* <span style="color: green;">[LIFECYCLE]</span> DRAFT PROPOSED PUBLISHED PROPOSED-DELETE Explain what the lifecycle of a package is.
* <span style="color: green;">[LATEST]</span> Explain the "latest" field works, and how it relates to the "latest" annotation
* <span style="color: green;">[REPOSITORIES]</span> GIT repo short hand explanation.
[WORKSPACE/(REVISION IDENTIFIER)] Explain what a workspace is****, see <https://github.com/nephio-project/nephio/issues/831>
* <span style="color: green;">[PACKAGE-RELATIONSHIPS]</span> UPSTREAM VS DOWNSTREAM VS Explain the behavior of packages which contain embedded packages <https://github.com/nephio-project/nephio/discussions/917>
* <span style="color: green;">[EXPLAIN UPSTREAM AT A HIGH LEVEL HERE BEFORE GOING IN DETAIL]</span>

</details>

---

## 3. Getting Started

**Section Must Contain:**

* Install Porch: requirements, supported platforms/environments, step‑by‑step install. Porch already has *Installing Porch*.
* Environment preparation: what users need locally, or on a cluster. (Porch has *Preparing the Environment*.)
* First example / quick start: minimal working example (e.g. a package, mutate, deploy)
* Using the Porch CLI / basic commands. Porch has *Using the Porch CLI tool*.

**Currently Available Resources:**

* [Installing Porch](https://docs.nephio.org/docs/porch/user-guides/install-porch/) needs quick version e.g. script containing (./scripts/setup-dev-env.sh + make run-in-kind)
* Preparing the Environment
* Using the Porch CLI tool

**Gaps / additions Required:**

* End-to-end quickstart walkthrough
* Output examples (logs/screenshots)
* Supported platforms/environment matrix

---

## 4. Tutorials & How‑to's

**Section Must Contain:**

* Common tasks with step‑by‑step instructions:
  * Authenticating with remote Git repositories.
  * Using private registries.
  * Running Porch in different environments (cloud, on‑prem, VMs). E.g. *Running Porch on GKE*.
* Advanced how‑tos: customizing the mutation pipeline, variant selection, function runner templating, etc.

**Currently Available Resources:**

* Authenticating with remote Git
* Using authenticated private registries
* Running Porch on GKE
* Mutation pipeline & function runner content (under Concepts)

**Gaps / additions Required:**

* Complete end-to-end sample with full mutation + deployment
* Real-world examples (multi-repo setups)
* CI/CD testing integration

<details>
<summary>More Detail on concepts requiring explanation</summary>

* <span style="color: green;">[EXPECT EXAMPLE SCRIPT HAVING DEPLOYED PORCH FOR THEM AS A FIRST TIME USER]</span>
* <span style="color: green;">[STEP 1: SETUP PORCH REPOSITORIES RESOURCE]</span> LIKELY FIRST STEP FROM A DEPLOYMENT OF PORCH TO USE IT
* <span style="color: green;">[FLOWCHART EXPLAINING FLOW E2E]</span> init → pull → locally do changes → push → proposed → approved/rejected → if rejected changes required then re proposed → if approved → becomes published/latest →
* <span style="color: green;">[CREATING FIRST PACKAGE]</span> INIT HOLLOW PKG -> PULL PKG LOCALLY FROM REPO -> MODIFY LOCALLY -> PUSH TO UPSTREAM -> PROPOSE FOR APPROVAL -> APPROVE TO UPSTREAM REPO E.G. SAMPLE
* <span style="color: green;">[UPGRADE EXAMPLES]</span> [ALL THE DIFF SCENARIOS] [THIS IS THE MOST COMPLEX PART] [IT NEEDS TO BE VERY SPECIFIC ON WHAT DO/DONT WE SUPPORT]
* <span style="color: green;">[CREATE A GENERIC PACKAGE AND RUN IT THROUGH THE DIFFERENT UPGRADES TO SHOW HOW THEY WORK AND CHANGE]</span>
* <span style="color: green;">in upgrade scenario</span> we expect that we have NEW BLUEPRINT IS PUBLISHED → DEPLOYMENT PACKAGE CAN BE UPGRADED IF IT WAS BASED ON THAT BLUEPRINT (AKA THE UPSTREAM OF THIS PACKAGE POINTS AT THAT BLUEPRINT). assuming 2 repositories
* <span style="color: green;">[RESOURCE MERGE]</span> IS A STRUCTURAL 3 WAY MERGE → HAS CONTEXT OF THE STRUCTURE OF THE FILES ->
* <span style="color: green;">[COPY MERGE]</span> IS A FILE REPLACEMENT STRATEGY → USEFUL WHEN YOU DONT NEED PORCH TO BE AWARE OF THE CONTENT OF THE FILES ESPECIALLY IF THERE IS CONTENT INSIDE THE FILES THAT DO NOT COMPLY WITH KUSTOMIZE.
  * <span style="color: green;">[OTHER STRATEGIES]</span> …

</details>

---

## 5. Architecture & Components

**Section Must Contain:**

* Overall architecture diagram
* Main components/modules of Porch (controllers, function runner, variant controller, etc.)
* Data flow and interaction: how packages move through system, lifecycle events, error paths, etc.
* Dependencies: e.g. what external services Porch relies on (Git, registry, Kubernetes APIs)

**Currently Available Resources:**

* Porch in the Nephio Architecture
* Individual component pages: Function Runner, Variant Controller, etc.

**Gaps / additions Required:**

* Single consolidated diagram of Porch system
* Component interaction maps
* Package lifecycle description and flow diagram

<details>
<summary>More Detail on concepts requiring explanation</summary>

* [PORCH-SERVER] PORCH SERVER SPECIFIC MAIN CHUNK FOR DETAIL HERE
  * [AGGREGATED API SERVER] HOW CERTAIN PORCH RESOURCES ARE SERVED AND HANDLED AKA NOT THROUGH CRDS BUT AGGR API
  * [REPO SYNC] ENSURES LOCAL DB/CR CACHE AND UPSTREAM REPO’S ARE KEPT IN SYNC
* [ENGINE] MAIN BRAIN/LOGIC USED IN PROCESSING PACKAGES
  * [CACHE SYSTEM]
    * [DB-CACHE] EXPLAIN DIFFERENCE IN OPERATION COMPARED TO OTHER (E.G. DB DOESNT PUSH TO REPO UNTIL APPROVED)
    * [CR-CACHE] EXPLAIN DIFFERENCE IN OPERATION COMPARED TO OTHER
* [FUNCTION-RUNNER] MAIN CHUNK OF DETAIL REGARDING THIS HERE
  * [TASK PIPELINE] MUTATORS VS VALIDATORS + IMAGES ETC Explain how the package mutation pipelines work and are triggered
    * [PIPELINE ORDER] <https://docs.nephio.org/docs/porch/package-mutation-pipeline-order/>
  * [POD TEMPLATING] <https://docs.nephio.org/docs/porch/function-runner-pod-templates/>
* [CONTROLLERS] MAIN CHUNK OF DETAIL REGARDING THIS HERE
  * [PKG VARIANT CONTROLLER] <https://docs.nephio.org/docs/porch/package-variant/>
* [GIT-REPO] MAIN CHUNK OF DETAIL REGARDING THIS HERE
  * [DEPLOYMENT VS NON DEPLOYMENT REPO] EXPLAIN
    * [4 WAYS PKG REV COMES INTO EXISTENCE], [UPSTREAM IS THE SOURCE OF THE CLONE]
    * [CREATED USING RPKG INIT/API] , [IN THE CASE THERE IS NO UPSTREAM]
    * [COPY FROM ANOTHER REV IN THE SAME PKG] ,[NO UPSTREAM?]
    * [CAN BE CLONED FROM ANOTHER PKG REV A NEW ] [HAS UPSTREAM]
    * [CAN BE LOADED FROM GIT] [DEPENDS ON WEATHER IT HAD A HAD A CLONE SOURCE OR NOT AT THE TIME]
  * [UPSTREAM] EXPLAIN PORCH INTERACTION WITH UPSTREAM REPO'S
  * [DOWNSTREAM] EXPLAIN PORCH INTERACTION WITH DOWNSTREAM REPO'S
* [PORCH-SPECIFIC-RESOURCES] SUMMARY OF MAIN RESOURCES E.G. PACKAGE-REVISIONS.
  * [PACKAGE-REVISION] MORE DETAIL HERE
  * [PACKAGE-REVISION-RESOURCES] MORE DETAIL HERE
  * [PACKAGE-REV] MORE DETAIL HERE
  * [REPOSITORIES] MORE DETAIL HERE
    * [GIT VS OCI] PORCH SUPPORTS THE CONCEPT OF MULTIPLE REPOS OCI IS EXPERIMENTAL. AN EXERNAL REPO IS AN IMPLEMENTATION OF PORCH REPOSTIORY INTERFACE WHICH STORES PKG REVISION ON AN EXTERNAL SYSTEM. TODAY THERE ARE 2 EXTERNAL REPO IMPLEMENTATIONS. THEY ARE GIT(FULLY SUPPORTED) & OCI(EXPERIMENTALLY). DEVELOPERS ARE FREE TO DESIGN AND IMPLEMENT NEW EXTERNAL REPOS TYPES IF THEY WISH E.G. DB INTERFACE
* [PACKAGE-VARIANTS/-SETS] MORE DETAIL HERE
* [PACKAGES] UNSURE IF THIS IS STILL USED?

</details>

---

## 6. Configuration & Deployment

**Section Must Contain:**

* Configuration options (config as data, configuration schema) — key settings, environment variables, flags. Porch has *Configuration as Data*.
* Deployment modes: how Porch can be deployed (cluster, single VM, etc.)
* Versioning and upgrades
* Authentication, authorization configuration (connecting to Git, registries)

**Currently Available Resources:**

* Configuration as Data
* Git & Registry Auth (under How-Tos)
* GKE Deployment Guide

**Gaps / additions Required:**

* Config file schema and field definitions
* Supported deployment topologies
* Upgrade instructions / versioning policy

<details>
<summary>More Detail on concepts requiring explanation</summary>

* [DEPLOYMENTS] HOW DO DEPLOY/INSTALL PORCH ON DIFFERENT ENV’S
  * [OFFICIAL DEPLOYMENT] <https://github.com/nephio-project/catalog/tree/main/nephio/core/porch>
  * [INSTALLING PORCH] <https://docs.nephio.org/docs/porch/user-guides/install-porch/>
  * [LOCAL DEV ENV DEPLOYMENT] <https://docs.nephio.org/docs/porch/contributors-guide/environment-setup/>
  * [DEV PROCESS] <https://docs.nephio.org/docs/porch/contributors-guide/dev-process/>
* [CONFIGURATION] DIFFERENT WAYS TO CONFIGURE PORCH
  * [DB/CR CACHE SETUPS] HOW TO CONFIGURE PORCH TO RUN WITH A DB CACHE VS THE DEFAULT CR CACHE
  * [REPOSITORY TYPES] PUBLIC VS PRIVATE REPO’S FOR KPT FUNCTIONS USED BY PORCH NOT REPOS WHERE PACKAGES ARE STORED!!!
    * [PUBLIC IMAGE REPOSITORIES] GCR OR KPT/DEV
    * [PRIVATE IMAGE REPOSITORIES] <https://docs.nephio.org/docs/porch/user-guides/git-authentication-config/>
    * [PRIVATE REPOSITORY TLS AUTH] <https://docs.nephio.org/docs/porch/user-guides/using-authenticated-private-registries/>
  * [CERT MANAGER] CONFIGURING PORCH TO USE CERT MANAGER FOR WEBHOOK HANDLING <<https://github.com/nephio-project/catalog/tree/main/nephio/optional/porch-cert-manager-webhook> See <https://github.com/nephio-project/nephio/issues/902>>

</details>

---

## 7. CLI / API / Reference

**Section Must Contain:**

* CLI tool reference: all commands, flags, examples
* APIs / CRDs / Resources: full spec for Porch‑specific Kubernetes resources, with fields, validation, defaulting
* Schema definitions or API versioning
* Configuration schema reference, file formats etc.

**Currently Available Resources:**

* CLI usage guide (basic) <https://docs.nephio.org/docs/porch/user-guides/porchctl-cli-guide/>

**Gaps / additions Required:**

* Full CLI command reference (flags, subcommands)
* CRD reference (e.g., PackageVariant, Repository)
* YAML schema definitions and validation docs

<details>
<summary>More Detail on concepts requiring explanation</summary>

* [CLI] largely already completed here <https://docs.nephio.org/docs/porch/user-guides/porchctl-cli-guide/>

</details>

---

## 8. Best Practices & Patterns

**Section Must Contain:**

* Recommendations for structuring packages, versions & variants
* How to design reusable templates/functions
* Performance / scaling tips (e.g. for large numbers of packages or functions)
* Operational guidance: monitoring, logging, health checks

**Currently Available Resources:**

* Not directly addressed

**Gaps / additions Required:**

* Package/variant organization patterns
* Best practices for reusable mutations/functions
* Monitoring/logging guides

<details>
<summary>More Detail on concepts requiring explanation</summary>

</details>

---

## 9. Troubleshooting & FAQ

**Section Must Contain:**

* Common problems & their solutions
* Error messages & diagnostic steps
* Debugging tips / tools
* FAQ: questions new users often ask

**Currently Available Resources:**

* None found

**Gaps / additions Required:**

* FAQ page
* Error resolution page
* CLI diagnostic/debugging guide

<details>
<summary>More Detail on concepts requiring explanation</summary>

</details>

---

## 10. Security & Compliance

**Section Must Contain:**

* Authentication & authorization: how Porch ensures secure access
* Secrets / credentials handling (for Git, registries, etc.)
* Security considerations for function runner / templates / untrusted code
* TLS, encryption in transit / at rest if applicable

**Currently Available Resources:**

* Git & Registry authentication (under How-Tos)

**Gaps / additions Required:**

* Security model for untrusted functions
* Secrets handling / rotation model
* RBAC requirements and guidance

<details>
<summary>More Detail on concepts requiring explanation</summary>

  [TLS IN CONTAINER REG'S]
  [GIT REG AUTH IN PORCH REPOSITORIES RESOUCES]
  [SELF SIGNED TLS IN PORCH SERVER]
  [WEBHOOKS AND RBAC]

</details>

---

## 11. Glossary

**Section Must Contain:**

* Define domain‑specific or technical terms used throughout the docs (variant, package orchestration, mutation, etc.)

**Currently Available Resources:**

* An old and likely in need of reconstruction glossary page was found here <https://docs.nephio.org/docs/glossary-abbreviations/>

**Gaps / additions Required:**

* Term definitions + cross-links

<details>
<summary>More Detail on concepts requiring explanation</summary>

</details>

---

## 12. Contributing

**Section Must Contain:**

* How to contribute (code, documentation)
* Developer setup (how to build and run Porch locally) — Porch has *Setting up a local environment*.
* Process for submitting changes, code review, governance

**Currently Available Resources:**

* Developer setup guide

**Gaps / additions Required:**

* CONTRIBUTING.md page with PR process
* Code style conventions
* Maintainer guide / governance model

<details>
<summary>More Detail on concepts requiring explanation</summary>

* [SIGNING CLA GUIDE/OTHER REQUIREMENTS ETC]
* [DEPLOY DEV ENV GUIDE]
  * [LOCAL PORCH SERVER]
    * [OPTIONS] CAN BASICALLY DESCRIBE THE settings in launch.json & settings.json
    * [DBCACHE] ...
    * [CRCACHE] ...
  * [IN POD DEPLOYMENT]
* [RUN TESTS LOCALLY]
* [CREATE PR PROCEDURE]
  * [COMMON PR GOTCHA'S] COULD BE COVERED BY A TEMPLATE

</details>

---

## 13. Release Notes / Changelog

**Section Must Contain:**

* What’s new in each release
* Breaking changes, deprecations
* Migration guides if necessary

**Currently Available Resources:**

* None found in public docs

**Gaps / additions Required:**

* Changelog per version
* Migration instructions
* Release tagging structure

<details>
<summary>More Detail on concepts requiring explanation</summary>

* [CAN POINT IN SOME WAY TO THE RELEASE IN PORCH HERE] <https://github.com/nephio-project/porch/releases>

</details>

---
