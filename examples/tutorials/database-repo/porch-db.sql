DROP TABLE IF EXISTS package_revisions;
DROP TABLE IF EXISTS packages;
DROP TABLE IF EXISTS repositories;

DROP TYPE IF EXISTS package_rev_lifecycle;

CREATE TABLE IF NOT EXISTS repositories (
    namespace  TEXT NOT NULL,
    repo_name  TEXT NOT NULL,
    updated    TIMESTAMPTZ,
    updatedby  TEXT NOT NULL,
    deployment BOOLEAN,
    PRIMARY KEY (namespace, repo_name)
);

CREATE TABLE IF NOT EXISTS packages (
    namespace    TEXT NOT NULL,
	repo_name   TEXT NOT NULL,
    package_name TEXT NOT NULL,
    updated      TIMESTAMPTZ NOT NULL,
    updatedby    TEXT NOT NULL,
    PRIMARY KEY (namespace, repo_name, package_name),
    CONSTRAINT fk_repository
        FOREIGN KEY (namespace, repo_name)
        REFERENCES repositories (namespace, repo_name)
        ON DELETE CASCADE
);

CREATE TYPE package_rev_lifecycle AS ENUM ('Draft', 'Proposed', 'Published', 'DeletionProposed');

CREATE TABLE IF NOT EXISTS package_revisions (
    namespace      TEXT NOT NULL,
	repo_name      TEXT NOT NULL,
    package_name   TEXT NOT NULL,
    package_rev    TEXT NOT NULL,
    workspace_name TEXT NOT NULL,
    updated        TIMESTAMPTZ NOT NULL,
    updatedby      TEXT NOT NULL,
	lifecycle      package_rev_lifecycle NOT NULL,
    resources      BYTEA,
    PRIMARY KEY (namespace, repo_name, package_name, package_rev),
    CONSTRAINT fk_package
        FOREIGN KEY (namespace, repo_name, package_name)
        REFERENCES packages (namespace, repo_name, package_name)
        ON DELETE CASCADE
);
