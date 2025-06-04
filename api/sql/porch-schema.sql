    DROP TABLE IF EXISTS resources;
    DROP TABLE IF EXISTS package_revisions;
    DROP TABLE IF EXISTS packages;
    DROP TABLE IF EXISTS repositories;

    CREATE TABLE IF NOT EXISTS repositories (
        k8s_name_space  TEXT NOT NULL CHECK (k8s_name_space <> ''),
        k8s_name        TEXT NOT NULL CHECK (k8s_name <> ''),
        directory       TEXT NOT NULL,
        default_ws_name TEXT NOT NULL,
        meta            TEXT NOT NULL,
        spec            TEXT NOT NULL,
        updated         TIMESTAMP,
        updatedby       TEXT NOT NULL,
        deployment      BOOLEAN,
        PRIMARY KEY (k8s_name_space, k8s_name)
    );

    CREATE TABLE IF NOT EXISTS packages (
        k8s_name_space TEXT NOT NULL CHECK (k8s_name_space <> ''),
        k8s_name       TEXT NOT NULL CHECK (k8s_name <> ''),
        repo_k8s_name  TEXT NOT NULL,
        package_path   TEXT NOT NULL,
        meta           TEXT NOT NULL,
        spec           TEXT NOT NULL,
        updated        TIMESTAMP NOT NULL,
        updatedby      TEXT NOT NULL,
        PRIMARY KEY (k8s_name_space, k8s_name),
        CONSTRAINT fk_repository
            FOREIGN KEY (k8s_name_space, repo_k8s_name)
            REFERENCES repositories (k8s_name_space, k8s_name)
            ON DELETE CASCADE
    );

    CREATE TABLE IF NOT EXISTS package_revisions (
        k8s_name_space   TEXT NOT NULL CHECK (k8s_name_space <> ''),
        k8s_name         TEXT NOT NULL CHECK (k8s_name <> ''),
        package_k8s_name TEXT NOT NULL,
        workspace        TEXT NOT NULL,
        revision         INTEGER NOT NULL,
        meta             TEXT NOT NULL,
        spec             TEXT NOT NULL,
        updated          TIMESTAMP NOT NULL,
        updatedby        TEXT NOT NULL,
        lifecycle        TEXT CHECK (lifecycle IN ('Draft', 'Proposed', 'Published', 'DeletionProposed')) NOT NULL,
        PRIMARY KEY (k8s_name_space, k8s_name),
        CONSTRAINT fk_package
            FOREIGN KEY (k8s_name_space, package_k8s_name)
            REFERENCES packages (k8s_name_space, k8s_name)
            ON DELETE CASCADE
    );

    CREATE TABLE IF NOT EXISTS resources (
        k8s_name_space TEXT NOT NULL CHECK (k8s_name_space <> ''),
        k8s_name       TEXT NOT NULL CHECK (k8s_name <> ''),
        revision       INTEGER NOT NULL,
        resource_key   TEXT NOT NULL CHECK (resource_key <> ''),
        resource_value TEXT NOT NULL,
        PRIMARY KEY (k8s_name_space, k8s_name, resource_key),
        CONSTRAINT fk_package_rev
            FOREIGN KEY (k8s_name_space, k8s_name)
            REFERENCES package_revisions (k8s_name_space, k8s_name)
            ON DELETE CASCADE
    );
