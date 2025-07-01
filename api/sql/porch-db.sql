/*
Copyright 2024-2025 The kpt and Nephio Authors

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

     http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/
DROP TABLE IF EXISTS resources;

DROP TABLE IF EXISTS package_revisions;
DROP FUNCTION IF EXISTS check_immutable_package_revisions_columns;
DROP TRIGGER IF EXISTS immutable_package_revisions_columns on package_revisions;

DROP TABLE IF EXISTS packages;
DROP FUNCTION IF EXISTS check_immutable_packages_columns;
DROP TRIGGER IF EXISTS immutable_packages_columns on packages;

DROP TABLE IF EXISTS repositories;
DROP FUNCTION IF EXISTS check_immutable_repositories_columns;
DROP TRIGGER IF EXISTS immutable_repositories_columns on repositories;

CREATE TABLE IF NOT EXISTS repositories (
    k8s_name_space  TEXT NOT NULL CHECK (k8s_name_space <> ''),
    k8s_name        TEXT NOT NULL CHECK (k8s_name <> ''),
    directory       TEXT NOT NULL,
    default_ws_name TEXT NOT NULL,
    meta            TEXT NOT NULL,
    spec            TEXT NOT NULL,
    updated         TIMESTAMP,
    updatedby       TEXT,
    deployment      BOOLEAN,
    PRIMARY KEY (k8s_name_space, k8s_name)
);

CREATE FUNCTION check_immutable_repositories_columns() RETURNS trigger
    LANGUAGE plpgsql AS
$BODY$
BEGIN
    IF NEW.directory <> OLD.directory OR NEW.default_ws_name <> OLD.default_ws_name THEN
        RAISE EXCEPTION 'update not allowed on immutable columns "directory" and "default_ws_name"';
    END IF;
    RETURN NEW;
END;
$BODY$;

CREATE TRIGGER immutable_repositories_columns
   BEFORE UPDATE ON repositories FOR EACH ROW
   EXECUTE PROCEDURE check_immutable_repositories_columns();

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

CREATE FUNCTION check_immutable_packages_columns() RETURNS trigger
    LANGUAGE plpgsql AS
$BODY$
BEGIN
    IF NEW.repo_k8s_name <> OLD.repo_k8s_name OR NEW.package_path <> OLD.package_path THEN
        RAISE EXCEPTION 'update not allowed on immutable columns "repo_k8s_name" and "package_path"';
    END IF;
    RETURN NEW;
END;
$BODY$;

CREATE TRIGGER immutable_packages_columns
   BEFORE UPDATE ON packages FOR EACH ROW
   EXECUTE PROCEDURE check_immutable_packages_columns();

CREATE TABLE IF NOT EXISTS package_revisions (
    k8s_name_space   TEXT NOT NULL CHECK (k8s_name_space <> ''),
    k8s_name         TEXT NOT NULL CHECK (k8s_name <> ''),
    package_k8s_name TEXT NOT NULL,
    revision         INTEGER NOT NULL,
    meta             TEXT NOT NULL,
    spec             TEXT NOT NULL,
    updated          TIMESTAMP NOT NULL,
    updatedby        TEXT NOT NULL,
    lifecycle        TEXT CHECK (lifecycle IN ('Draft', 'Proposed', 'Published', 'DeletionProposed')) NOT NULL,
    tasks            TEXT NOT NULL,
    PRIMARY KEY (k8s_name_space, k8s_name),
    CONSTRAINT fk_package
        FOREIGN KEY (k8s_name_space, package_k8s_name)
        REFERENCES packages (k8s_name_space, k8s_name)
        ON DELETE CASCADE
);

CREATE FUNCTION check_immutable_package_revisions_columns() RETURNS trigger
    LANGUAGE plpgsql AS
$BODY$
BEGIN
    IF NEW.package_k8s_name <> OLD.package_k8s_name THEN
        RAISE EXCEPTION 'update not allowed on immutable column "package_k8s_name"';
    END IF;

    IF NEW.revision != OLD.revision THEN
        IF NEW.lifecycle = OLD.lifecycle THEN
            RAISE EXCEPTION 'update not allowed on column "revision", lifecycle has not changed';
        ELSIF NEW.lifecycle = 'Draft' OR NEW.lifecycle = 'Proposed' OR NEW.lifecycle = 'DeletionProposed' THEN
            RAISE EXCEPTION 'update not allowed on column "revision", new lifecycle must be "Published"';
        END IF;
    END IF;

    RETURN NEW;
END;
$BODY$;

CREATE TRIGGER immutable_package_revisions_columns
   BEFORE UPDATE ON package_revisions FOR EACH ROW
   EXECUTE PROCEDURE check_immutable_package_revisions_columns();

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
