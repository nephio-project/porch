---
title: "Nephio Architecture"
type: docs
weight: 5
description: Reference for the Nephio Architecture
---

<div style="border: 1px solid red; background-color: #ffe6e6; color: #b30000; padding: 1em; margin-bottom: 1em;">
  <strong>⚠️ Outdated Notice:</strong> This page refers to an older version of the documentation. This content has simply been moved into its relevant new section here and must be checked, modified, rewritten, updated, or removed entirely.
</div>

Some experiments on working with [C4 model](https://c4model.com/) to document Nephio.

## Prerequisites
1. [Graphviz](https://graphviz.org/download/) is required to render some of the diagrams in this document.

## System Context View


![System Context](/static/images/architecture/level1-nephio-system.png)

The system context view gives a high level perspective of the Nephio software system and the external entities that it interacts with. There are no deployment considerations in this view - the main purpose of the picture is to depict what is the responsibility and scope of Nephio, and the key interfaces and capabilities it exposes to deliver on that responsibility.

## System Landscape View

![System Landscape](/static/images/architecture/level2-nephio-container.png)

Nephio is an amalgamation of software systems, so a system landscape provides a high-level view of how those software systems operate together.

## Component Views

### Nephio Core

![Nephio Core Component View](/static/images/architecture/level3-nephio-core-component.png)

Nephio core is a collection of operators and functions that perform the fundamental aspects of Nephio use cases, independent of the specifics of vendor implementations. 

The controllers for OAI and Free5GC are represented here. Although they are vendor extensions to Nephio, they are for now part of the Nephio system.


### Porch

![Nephio Porch Component View](/static/images/architecture/nephio-porch-component-view.png)
