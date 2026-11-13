-- Tables as Gogs created them on a new installation at schema version 22. Change
-- the schema with new migrations, never by editing this file.

CREATE TABLE "access" (
    "id" BIGSERIAL,
    "user_id" bigint NOT NULL,
    "repo_id" bigint NOT NULL,
    "mode" bigint NOT NULL,
    CONSTRAINT "access_pkey" PRIMARY KEY (id)
);

CREATE UNIQUE INDEX access_user_repo_unique ON access (user_id, repo_id);

CREATE TABLE "access_token" (
    "id" BIGSERIAL,
    "uid" bigint,
    "name" text,
    "sha1" character varying(40),
    "sha256" character varying(64) NOT NULL,
    "created_unix" bigint,
    "updated_unix" bigint,
    CONSTRAINT "access_token_pkey" PRIMARY KEY (id),
    CONSTRAINT "uni_access_token_sha1" UNIQUE (sha1),
    CONSTRAINT "uni_access_token_sha256" UNIQUE (sha256)
);

CREATE INDEX idx_access_token_user_id ON access_token (uid);

CREATE TABLE "action" (
    "id" BIGSERIAL,
    "user_id" bigint,
    "op_type" bigint,
    "act_user_id" bigint,
    "act_user_name" text,
    "repo_id" bigint,
    "repo_user_name" text,
    "repo_name" text,
    "ref_name" text,
    "is_private" boolean DEFAULT false NOT NULL,
    "content" text,
    "created_unix" bigint,
    CONSTRAINT "action_pkey" PRIMARY KEY (id)
);

CREATE INDEX idx_action_repo_id ON action (repo_id);

CREATE INDEX idx_action_user_id ON action (user_id);

CREATE TABLE "attachment" (
    "id" BIGSERIAL,
    "uuid" uuid,
    "issue_id" bigint,
    "comment_id" bigint,
    "release_id" bigint,
    "name" character varying(255),
    "created_unix" bigint,
    CONSTRAINT "attachment_pkey" PRIMARY KEY (id)
);

CREATE INDEX "IDX_attachment_issue_id" ON attachment (issue_id);

CREATE INDEX "IDX_attachment_release_id" ON attachment (release_id);

CREATE UNIQUE INDEX "UQE_attachment_uuid" ON attachment (uuid);

CREATE TABLE "collaboration" (
    "id" BIGSERIAL,
    "user_id" bigint NOT NULL,
    "repo_id" bigint NOT NULL,
    "mode" integer DEFAULT 2 NOT NULL,
    CONSTRAINT "collaboration_pkey" PRIMARY KEY (id)
);

CREATE INDEX "IDX_collaboration_repo_id" ON collaboration (repo_id);

CREATE INDEX "IDX_collaboration_user_id" ON collaboration (user_id);

CREATE UNIQUE INDEX "UQE_collaboration_s" ON collaboration (user_id, repo_id);

CREATE TABLE "comment" (
    "id" BIGSERIAL,
    "type" integer,
    "poster_id" bigint,
    "issue_id" bigint,
    "commit_id" bigint,
    "line" bigint,
    "content" text,
    "created_unix" bigint,
    "updated_unix" bigint,
    "commit_sha" character varying(40),
    CONSTRAINT "comment_pkey" PRIMARY KEY (id)
);

CREATE INDEX "IDX_comment_issue_id" ON comment (issue_id);

CREATE TABLE "deploy_key" (
    "id" BIGSERIAL,
    "key_id" bigint,
    "repo_id" bigint,
    "name" character varying(255),
    "fingerprint" character varying(255),
    "created_unix" bigint,
    "updated_unix" bigint,
    CONSTRAINT "deploy_key_pkey" PRIMARY KEY (id)
);

CREATE INDEX "IDX_deploy_key_key_id" ON deploy_key (key_id);

CREATE INDEX "IDX_deploy_key_repo_id" ON deploy_key (repo_id);

CREATE UNIQUE INDEX "UQE_deploy_key_s" ON deploy_key (key_id, repo_id);

CREATE TABLE "email_address" (
    "id" BIGSERIAL,
    "uid" bigint NOT NULL,
    "email" character varying(254) NOT NULL,
    "is_activated" boolean DEFAULT false NOT NULL,
    CONSTRAINT "email_address_pkey" PRIMARY KEY (id)
);

CREATE UNIQUE INDEX email_address_user_email_unique ON email_address (uid, email);

CREATE INDEX idx_email_address_user_id ON email_address (uid);

CREATE TABLE "follow" (
    "id" BIGSERIAL,
    "user_id" bigint NOT NULL,
    "follow_id" bigint NOT NULL,
    CONSTRAINT "follow_pkey" PRIMARY KEY (id)
);

CREATE UNIQUE INDEX follow_user_follow_unique ON follow (user_id, follow_id);

CREATE TABLE "hook_task" (
    "id" BIGSERIAL,
    "repo_id" bigint,
    "hook_id" bigint,
    "uuid" character varying(255),
    "type" integer,
    "url" text,
    "signature" text,
    "payload_content" text,
    "content_type" integer,
    "event_type" character varying(255),
    "is_ssl" boolean,
    "is_delivered" boolean,
    "delivered" bigint,
    "is_succeed" boolean,
    "request_content" text,
    "response_content" text,
    CONSTRAINT "hook_task_pkey" PRIMARY KEY (id)
);

CREATE INDEX "IDX_hook_task_repo_id" ON hook_task (repo_id);

CREATE TABLE "issue" (
    "id" BIGSERIAL,
    "repo_id" bigint,
    "index" bigint,
    "poster_id" bigint,
    "name" character varying(255),
    "content" text,
    "milestone_id" bigint,
    "priority" integer,
    "assignee_id" bigint,
    "is_closed" boolean,
    "is_pull" boolean,
    "num_comments" integer,
    "deadline_unix" bigint,
    "created_unix" bigint,
    "updated_unix" bigint,
    CONSTRAINT "issue_pkey" PRIMARY KEY (id)
);

CREATE INDEX "IDX_issue_repo_id" ON issue (repo_id);

CREATE UNIQUE INDEX "UQE_issue_repo_index" ON issue (repo_id, index);

CREATE TABLE "issue_label" (
    "id" BIGSERIAL,
    "issue_id" bigint,
    "label_id" bigint,
    CONSTRAINT "issue_label_pkey" PRIMARY KEY (id)
);

CREATE UNIQUE INDEX "UQE_issue_label_s" ON issue_label (issue_id, label_id);

CREATE TABLE "issue_user" (
    "id" BIGSERIAL,
    "uid" bigint,
    "issue_id" bigint,
    "repo_id" bigint,
    "milestone_id" bigint,
    "is_read" boolean,
    "is_assigned" boolean,
    "is_mentioned" boolean,
    "is_poster" boolean,
    "is_closed" boolean,
    CONSTRAINT "issue_user_pkey" PRIMARY KEY (id)
);

CREATE INDEX "IDX_issue_user_repo_id" ON issue_user (repo_id);

CREATE INDEX "IDX_issue_user_uid" ON issue_user (uid);

CREATE TABLE "label" (
    "id" BIGSERIAL,
    "repo_id" bigint,
    "name" character varying(255),
    "color" character varying(7),
    "num_issues" integer,
    "num_closed_issues" integer,
    CONSTRAINT "label_pkey" PRIMARY KEY (id)
);

CREATE INDEX "IDX_label_repo_id" ON label (repo_id);

CREATE TABLE "lfs_object" (
    "repo_id" bigint NOT NULL,
    "oid" text NOT NULL,
    "size" bigint NOT NULL,
    "storage" text NOT NULL,
    "created_at" timestamp with time zone NOT NULL,
    CONSTRAINT "lfs_object_pkey" PRIMARY KEY (repo_id, oid)
);

CREATE TABLE "login_source" (
    "id" BIGSERIAL,
    "type" bigint,
    "name" text,
    "is_actived" boolean NOT NULL,
    "is_default" boolean,
    "cfg" text,
    "created_unix" bigint,
    "updated_unix" bigint,
    CONSTRAINT "login_source_pkey" PRIMARY KEY (id),
    CONSTRAINT "uni_login_source_name" UNIQUE (name)
);

CREATE TABLE "milestone" (
    "id" BIGSERIAL,
    "repo_id" bigint,
    "name" character varying(255),
    "content" text,
    "is_closed" boolean,
    "num_issues" integer,
    "num_closed_issues" integer,
    "completeness" integer,
    "deadline_unix" bigint,
    "closed_date_unix" bigint,
    CONSTRAINT "milestone_pkey" PRIMARY KEY (id)
);

CREATE INDEX "IDX_milestone_repo_id" ON milestone (repo_id);

CREATE TABLE "mirror" (
    "id" BIGSERIAL,
    "repo_id" bigint,
    "interval" integer,
    "enable_prune" boolean DEFAULT true NOT NULL,
    "updated_unix" bigint,
    "next_update_unix" bigint,
    CONSTRAINT "mirror_pkey" PRIMARY KEY (id)
);

CREATE TABLE "notice" (
    "id" BIGSERIAL,
    "type" bigint,
    "description" text,
    "created_unix" bigint,
    CONSTRAINT "notice_pkey" PRIMARY KEY (id)
);

CREATE TABLE "org_user" (
    "id" BIGSERIAL,
    "uid" bigint,
    "org_id" bigint,
    "is_public" boolean,
    "is_owner" boolean,
    "num_teams" integer,
    CONSTRAINT "org_user_pkey" PRIMARY KEY (id)
);

CREATE INDEX "IDX_org_user_org_id" ON org_user (org_id);

CREATE INDEX "IDX_org_user_uid" ON org_user (uid);

CREATE UNIQUE INDEX "UQE_org_user_s" ON org_user (uid, org_id);

CREATE TABLE "protect_branch" (
    "id" BIGSERIAL,
    "repo_id" bigint,
    "name" character varying(255),
    "protected" boolean,
    "require_pull_request" boolean,
    "enable_whitelist" boolean,
    "whitelist_user_i_ds" text,
    "whitelist_team_i_ds" text,
    CONSTRAINT "protect_branch_pkey" PRIMARY KEY (id)
);

CREATE UNIQUE INDEX "UQE_protect_branch_protect_branch" ON protect_branch (repo_id, name);

CREATE TABLE "protect_branch_whitelist" (
    "id" BIGSERIAL,
    "protect_branch_id" bigint,
    "repo_id" bigint,
    "name" character varying(255),
    "user_id" bigint,
    CONSTRAINT "protect_branch_whitelist_pkey" PRIMARY KEY (id)
);

CREATE UNIQUE INDEX "UQE_protect_branch_whitelist_protect_branch_whitelist" ON protect_branch_whitelist (repo_id, name, user_id);

CREATE TABLE "public_key" (
    "id" BIGSERIAL,
    "owner_id" bigint NOT NULL,
    "name" character varying(255) NOT NULL,
    "fingerprint" character varying(255) NOT NULL,
    "content" text NOT NULL,
    "mode" integer DEFAULT 2 NOT NULL,
    "type" integer DEFAULT 1 NOT NULL,
    "created_unix" bigint,
    "updated_unix" bigint,
    CONSTRAINT "public_key_pkey" PRIMARY KEY (id)
);

CREATE INDEX "IDX_public_key_owner_id" ON public_key (owner_id);

CREATE TABLE "pull_request" (
    "id" BIGSERIAL,
    "type" integer,
    "status" integer,
    "issue_id" bigint,
    "index" bigint,
    "head_repo_id" bigint,
    "base_repo_id" bigint,
    "head_user_name" character varying(255),
    "head_branch" character varying(255),
    "base_branch" character varying(255),
    "merge_base" character varying(40),
    "has_merged" boolean,
    "merged_commit_id" character varying(40),
    "merger_id" bigint,
    "merged_unix" bigint,
    CONSTRAINT "pull_request_pkey" PRIMARY KEY (id)
);

CREATE INDEX "IDX_pull_request_issue_id" ON pull_request (issue_id);

CREATE TABLE "release" (
    "id" BIGSERIAL,
    "repo_id" bigint,
    "publisher_id" bigint,
    "tag_name" character varying(255),
    "lower_tag_name" character varying(255),
    "target" character varying(255),
    "title" character varying(255),
    "sha1" character varying(40),
    "num_commits" bigint,
    "note" text,
    "is_draft" boolean DEFAULT false NOT NULL,
    "is_prerelease" boolean,
    "created_unix" bigint,
    CONSTRAINT "release_pkey" PRIMARY KEY (id)
);

CREATE TABLE "repository" (
    "id" BIGSERIAL,
    "owner_id" bigint,
    "lower_name" character varying(255) NOT NULL,
    "name" character varying(255) NOT NULL,
    "description" character varying(512),
    "website" character varying(255),
    "default_branch" character varying(255),
    "size" bigint DEFAULT 0 NOT NULL,
    "use_custom_avatar" boolean,
    "num_watches" integer,
    "num_stars" integer,
    "num_forks" integer,
    "num_issues" integer,
    "num_closed_issues" integer,
    "num_pulls" integer,
    "num_closed_pulls" integer,
    "num_milestones" integer DEFAULT 0 NOT NULL,
    "num_closed_milestones" integer DEFAULT 0 NOT NULL,
    "is_private" boolean,
    "is_unlisted" boolean DEFAULT false NOT NULL,
    "is_bare" boolean,
    "is_mirror" boolean,
    "enable_wiki" boolean DEFAULT true NOT NULL,
    "allow_public_wiki" boolean,
    "enable_external_wiki" boolean,
    "external_wiki_url" character varying(255),
    "enable_issues" boolean DEFAULT true NOT NULL,
    "allow_public_issues" boolean,
    "enable_external_tracker" boolean,
    "external_tracker_url" character varying(255),
    "external_tracker_format" character varying(255),
    "external_tracker_style" character varying(255),
    "enable_pulls" boolean DEFAULT true NOT NULL,
    "pulls_ignore_whitespace" boolean DEFAULT false NOT NULL,
    "pulls_allow_rebase" boolean DEFAULT false NOT NULL,
    "is_fork" boolean DEFAULT false NOT NULL,
    "fork_id" bigint,
    "created_unix" bigint,
    "updated_unix" bigint,
    CONSTRAINT "repository_pkey" PRIMARY KEY (id)
);

CREATE INDEX "IDX_repository_lower_name" ON repository (lower_name);

CREATE INDEX "IDX_repository_name" ON repository (name);

CREATE UNIQUE INDEX "UQE_repository_s" ON repository (owner_id, lower_name);

CREATE TABLE "star" (
    "id" BIGSERIAL,
    "uid" bigint,
    "repo_id" bigint,
    CONSTRAINT "star_pkey" PRIMARY KEY (id)
);

CREATE UNIQUE INDEX "UQE_star_s" ON star (uid, repo_id);

CREATE TABLE "team" (
    "id" BIGSERIAL,
    "org_id" bigint,
    "lower_name" character varying(255),
    "name" character varying(255),
    "description" character varying(255),
    "authorize" integer,
    "num_repos" integer,
    "num_members" integer,
    CONSTRAINT "team_pkey" PRIMARY KEY (id)
);

CREATE INDEX "IDX_team_org_id" ON team (org_id);

CREATE TABLE "team_repo" (
    "id" BIGSERIAL,
    "org_id" bigint,
    "team_id" bigint,
    "repo_id" bigint,
    CONSTRAINT "team_repo_pkey" PRIMARY KEY (id)
);

CREATE INDEX "IDX_team_repo_org_id" ON team_repo (org_id);

CREATE UNIQUE INDEX "UQE_team_repo_s" ON team_repo (team_id, repo_id);

CREATE TABLE "team_user" (
    "id" BIGSERIAL,
    "org_id" bigint,
    "team_id" bigint,
    "uid" bigint,
    CONSTRAINT "team_user_pkey" PRIMARY KEY (id)
);

CREATE INDEX "IDX_team_user_org_id" ON team_user (org_id);

CREATE UNIQUE INDEX "UQE_team_user_s" ON team_user (team_id, uid);

CREATE TABLE "two_factor" (
    "id" BIGSERIAL,
    "user_id" bigint,
    "secret" character varying(255),
    "created_unix" bigint,
    CONSTRAINT "two_factor_pkey" PRIMARY KEY (id)
);

CREATE UNIQUE INDEX "UQE_two_factor_user_id" ON two_factor (user_id);

CREATE TABLE "two_factor_recovery_code" (
    "id" BIGSERIAL,
    "user_id" bigint,
    "code" character varying(11),
    "is_used" boolean,
    CONSTRAINT "two_factor_recovery_code_pkey" PRIMARY KEY (id)
);

CREATE TABLE "upload" (
    "id" BIGSERIAL,
    "uuid" uuid,
    "name" character varying(255),
    CONSTRAINT "upload_pkey" PRIMARY KEY (id)
);

CREATE UNIQUE INDEX "UQE_upload_uuid" ON upload (uuid);

CREATE TABLE "user" (
    "id" BIGSERIAL,
    "lower_name" character varying(255) NOT NULL,
    "name" character varying(255) NOT NULL,
    "full_name" character varying(255),
    "email" character varying(255) NOT NULL,
    "passwd" character varying(255) NOT NULL,
    "login_source" bigint DEFAULT 0 NOT NULL,
    "login_name" character varying(255),
    "type" integer,
    "location" character varying(255),
    "website" character varying(255),
    "rands" character varying(10),
    "salt" character varying(10),
    "created_unix" bigint,
    "updated_unix" bigint,
    "last_repo_visibility" boolean,
    "max_repo_creation" integer DEFAULT '-1'::integer NOT NULL,
    "is_active" boolean,
    "is_admin" boolean,
    "allow_git_hook" boolean,
    "allow_import_local" boolean,
    "prohibit_login" boolean,
    "avatar" character varying(2048) NOT NULL,
    "avatar_email" character varying(255) NOT NULL,
    "use_custom_avatar" boolean,
    "num_followers" integer,
    "num_following" integer DEFAULT 0 NOT NULL,
    "num_stars" integer,
    "num_repos" integer,
    "description" character varying(255),
    "num_teams" integer,
    "num_members" integer,
    CONSTRAINT "user_pkey" PRIMARY KEY (id)
);

CREATE UNIQUE INDEX "UQE_user_lower_name" ON "user" (lower_name);

CREATE UNIQUE INDEX "UQE_user_name" ON "user" (name);

CREATE TABLE "watch" (
    "id" BIGSERIAL,
    "user_id" bigint,
    "repo_id" bigint,
    CONSTRAINT "watch_pkey" PRIMARY KEY (id)
);

CREATE UNIQUE INDEX "UQE_watch_watch" ON watch (user_id, repo_id);

CREATE TABLE "webhook" (
    "id" BIGSERIAL,
    "repo_id" bigint,
    "org_id" bigint,
    "url" text,
    "content_type" integer,
    "secret" text,
    "events" text,
    "is_ssl" boolean,
    "is_active" boolean,
    "hook_task_type" integer,
    "meta" text,
    "last_status" integer,
    "created_unix" bigint,
    "updated_unix" bigint,
    CONSTRAINT "webhook_pkey" PRIMARY KEY (id)
);
