DROP TABLE IF EXISTS resources;
DROP TABLE IF EXISTS package_revisions;
DROP TABLE IF EXISTS packages;
DROP TABLE IF EXISTS repositories;

DROP TYPE IF EXISTS package_rev_lifecycle;

CREATE TABLE IF NOT EXISTS repositories (
    name_space TEXT NOT NULL,
    repo_name  TEXT NOT NULL,
    meta       TEXT NOT NULL,
    spec       TEXT NOT NULL,
    updated    TIMESTAMPTZ,
    updatedby  TEXT NOT NULL,
    deployment BOOLEAN,
    PRIMARY KEY (name_space, repo_name)
);

CREATE TABLE IF NOT EXISTS packages (
    name_space   TEXT NOT NULL,
	repo_name    TEXT NOT NULL,
    package_name TEXT NOT NULL,
    meta         TEXT NOT NULL,
    spec         TEXT NOT NULL,
    updated      TIMESTAMPTZ NOT NULL,
    updatedby    TEXT NOT NULL,
    PRIMARY KEY (name_space, repo_name, package_name),
    CONSTRAINT fk_repository
        FOREIGN KEY (name_space, repo_name)
        REFERENCES repositories (name_space, repo_name)
        ON DELETE CASCADE
);

CREATE TYPE package_rev_lifecycle AS ENUM ('Draft', 'Proposed', 'Published', 'DeletionProposed');

CREATE TABLE IF NOT EXISTS package_revisions (
    name_space     TEXT NOT NULL,
	repo_name      TEXT NOT NULL,
    package_name   TEXT NOT NULL,
    workspace_name TEXT NOT NULL,
    package_rev    INTEGER NOT NULL,
    meta           TEXT NOT NULL,
    spec           TEXT NOT NULL,
    updated        TIMESTAMPTZ NOT NULL,
    updatedby      TEXT NOT NULL,
	lifecycle      package_rev_lifecycle NOT NULL,
    PRIMARY KEY (name_space, repo_name, package_name, workspace_name),
    CONSTRAINT fk_package
        FOREIGN KEY (name_space, repo_name, package_name)
        REFERENCES packages (name_space, repo_name, package_name)
        ON DELETE CASCADE
);

CREATE TABLE IF NOT EXISTS resources (
    name_space     TEXT NOT NULL,
	repo_name      TEXT NOT NULL,
    package_name   TEXT NOT NULL,
    workspace_name TEXT NOT NULL,
    package_rev    INTEGER NOT NULL,
    resource_key   TEXT NOT NULL,
    resource_value TEXT NOT NULL,
    PRIMARY KEY (name_space, repo_name, package_name, workspace_name, resource_key),
    CONSTRAINT fk_package_rev
        FOREIGN KEY (name_space, repo_name, package_name, workspace_name)
        REFERENCES package_revisions (name_space, repo_name, package_name, workspace_name)
        ON DELETE CASCADE
);
