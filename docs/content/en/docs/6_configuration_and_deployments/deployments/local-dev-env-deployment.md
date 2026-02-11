---
title: "Local Development Environment Setup"
type: docs
weight: 3
description: "A guide to setting up a local environment for developing and testing with Porch."
---

# Local Development Environment Setup

This guide provides instructions for setting up a local development environment using `kind` (Kubernetes in Docker). This setup is ideal for developing, testing, and exploring Porch functionalities.

## Table of Contents

- [Prerequisites](#prerequisites)
- [Local Environment Setup](#local-environment-setup)
- [Verifying the Setup](#verifying-the-setup)

## Prerequisites

Before you begin, ensure you have the following tools installed on your system:

*   **[Docker](https://docs.docker.com/get-docker/):** For running containers, including the `kind` cluster.
*   **[kubectl](https://kubernetes.io/docs/tasks/tools/install-kubectl/):** The Kubernetes command-line tool for interacting with your cluster.
*   **[kind](https://kind.sigs.k8s.io/docs/user/quick-start/#installation):** A tool for running local Kubernetes clusters using Docker container "nodes".

The setup scripts provided in the Porch repository will handle the installation of Porch itself and its CLI, `porchctl`.

## Local Environment Setup

Follow these steps from the root directory of your cloned Porch repository to set up your local environment.

1.  **Bring up the `kind` cluster:**

    This script creates a local Kubernetes cluster with the necessary configuration for Porch.

    ```bash
    ./scripts/setup-dev-env.sh
    ```

2.  **Build and load Porch images:**

    **Choose one of the following options** to build the Porch container images and load them into your `kind` cluster.

    *   **CR-CACHE (Default):** Uses a cache backed by a Custom Resource (CR).
        ```bash
        make run-in-kind
        ```

    *   **DB-CACHE:** Uses a PostgreSQL database as the cache backend.
        ```bash
        make run-in-kind-db-cache
        ```

## Verifying the Setup

After the setup scripts complete, verify that all components are running correctly.

1.  **Check Pod Status:**

    Ensure all pods in the `porch-system` namespace are in the `READY` state.

    ```bash
    kubectl get pods -n porch-system
    ```

2.  **Verify CRD Availability:**

    Confirm that the `PackageRevision` Custom Resource Definition (CRD) has been successfully registered.

    ```bash
    kubectl api-resources | grep packagerevisions
    ```

3.  **Configure `porchctl` (Optional):**

    The `porchctl` binary is built into the `.build/` directory. For convenient access, add it to your system's `PATH`.

    ```bash
    # You can copy the binary to a directory in your PATH, for example:
    sudo cp ./.build/porchctl /usr/local/bin/porchctl

    # Alternatively, you can add the build directory to your PATH:
    export PATH="$(pwd)/.build:$PATH"
    ```

4.  **Access Gitea UI (Optional):**

    The local environment includes a Gitea instance for Git repository hosting. You can access it at [http://localhost:3000](http://localhost:3000).

    *   **Username:** `nephio`
    *   **Password:** `secret`
