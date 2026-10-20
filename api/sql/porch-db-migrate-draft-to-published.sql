/*
Copyright 2026 The kpt Authors

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

     http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.

Upgrade: allow assigning revision on publish when OLD.lifecycle is Draft (direct
Draft -> Published), not only Proposed.

Apply on existing PostgreSQL volumes before using direct publication. Fresh
installs that run api/sql/porch-db.sql via docker-entrypoint-initdb.d already
include this logic; init scripts are not re-run on an initialized data directory.

Usage:
  PGPASSWORD=<password> psql -h <host> -U <user> -d <database> \
    -v ON_ERROR_STOP=1 -f api/sql/porch-db-migrate-draft-to-published.sql

Verify (should match "is not Draft or Proposed", not "is not Proposed"):
  SELECT prosrc FROM pg_proc WHERE proname = 'check_package_revisions_columns';
*/

CREATE OR REPLACE FUNCTION check_package_revisions_columns() RETURNS trigger
    LANGUAGE plpgsql AS
$BODY$
DECLARE
    lifecycle_draft             CONSTANT VARCHAR(5)  := 'Draft';
    lifecycle_proposed          CONSTANT VARCHAR(8)  := 'Proposed';
    lifecycle_published         CONSTANT VARCHAR(9)  := 'Published';
    lifecycle_deletion_proposed CONSTANT VARCHAR(16) := 'DeletionProposed';

    count_revisions INTEGER;
    latest_revision INTEGER;
BEGIN
    IF NEW.package_k8s_name != OLD.package_k8s_name THEN
        RAISE EXCEPTION 'create or update not allowed on immutable column "package_k8s_name"';
    END IF;

    IF NEW.revision < -1 OR OLD.revision < -1 THEN
        RAISE EXCEPTION 'create or update not allowed on column "revision", revisions of less than -1 are not allowed';
    END IF;

    -- Package revisions in external repositories with uncontrolled revisions must always be 'Published' and cannot have lifecycle changes
    IF NEW.revision = -1 OR OLD.revision = -1 THEN
        IF NOT (NEW.lifecycle = lifecycle_published OR NEW.lifecycle = lifecycle_deletion_proposed) OR NOT (OLD.lifecycle = lifecycle_published OR OLD.lifecycle = lifecycle_deletion_proposed) THEN
            RAISE EXCEPTION 'create or update not allowed on column "lifecycle", lifecycle of % illegal, package revision has revision -1', NEW.lifecycle;
        ELSE
            return NEW;
        END IF;
    END IF;

    -- Package revisions with revision 0 are draft package revisions
    IF NEW.revision = 0 THEN
        IF OLD.revision != 0 THEN
            RAISE EXCEPTION 'create or update not allowed on column "revision", update of revision from % to zero is illegal', OLD.revision;
        END IF;

        IF NOT (NEW.lifecycle = lifecycle_draft OR NEW.lifecycle = lifecycle_proposed) OR NOT (OLD.lifecycle = lifecycle_draft OR OLD.lifecycle = lifecycle_proposed) THEN
            RAISE EXCEPTION 'create or update not allowed on column "revision", revision value of 0 is only allowed on when lifecycle is Draft or Proposed';
        END IF;

        return NEW;
    END IF;

    IF NEW.lifecycle = lifecycle_draft THEN
        RAISE EXCEPTION 'create or update not allowed on column "revision", revision of % on drafts is illegal', NEW.revision;
    END IF;

    IF NEW.revision = OLD.revision THEN
        IF NEW.lifecycle = OLD.lifecycle THEN
            return NEW;
        END IF;

        IF NEW.lifecycle = lifecycle_published OR NEW.lifecycle = lifecycle_deletion_proposed THEN
            return NEW;
        END IF;

        RAISE EXCEPTION 'create or update not allowed on column "lifecycle", change from % to % is illegal', OLD.lifecycle, NEW.lifecycle;
    END IF;

    IF OLD.revision != 0 THEN
        RAISE EXCEPTION 'create or update not allowed on column "revision", update of revision from % to % is illegal', OLD.revision, NEW.revision;
    END IF;

    IF OLD.lifecycle != lifecycle_proposed AND OLD.lifecycle != lifecycle_draft THEN
        RAISE EXCEPTION 'create or update not allowed on column "revision", lifecycle % is not Draft or Proposed', OLD.lifecycle;
    END IF;

    count_revisions := (SELECT COUNT(revision) FROM package_revisions WHERE k8s_name_space = NEW.k8s_name_space AND package_k8s_name = NEW.package_k8s_name AND revision = NEW.revision);
    IF count_revisions > 0 THEN
       RAISE EXCEPTION 'create or update not allowed on column "revision", revision % already exists', NEW.revision;
    END IF;

    latest_revision := (SELECT MAX(revision) FROM package_revisions WHERE k8s_name_space = NEW.k8s_name_space AND package_k8s_name = NEW.package_k8s_name AND revision > 0);

    IF NEW.lifecycle = lifecycle_published AND NEW.revision > 0 AND (latest_revision IS NULL OR NEW.revision >= latest_revision) THEN
        UPDATE package_revisions SET latest = FALSE WHERE k8s_name_space = NEW.k8s_name_space AND package_k8s_name = NEW.package_k8s_name AND latest;
        NEW.latest = TRUE;
    END IF;

    RETURN NEW;
END;
$BODY$;
