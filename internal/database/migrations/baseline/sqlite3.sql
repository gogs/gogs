-- Tables as Gogs created them on a new installation at schema version 22. Change
-- the schema with new migrations, never by editing this file.

CREATE TABLE `access` (
    `id` integer PRIMARY KEY AUTOINCREMENT,
    `user_id` integer NOT NULL,
    `repo_id` integer NOT NULL,
    `mode` integer NOT NULL
);

CREATE UNIQUE INDEX `access_user_repo_unique` ON `access`(`user_id`,`repo_id`);

CREATE TABLE `access_token` (
    `id` integer PRIMARY KEY AUTOINCREMENT,
    `uid` integer,
    `name` text,
    `sha1` VARCHAR(40),
    `sha256` VARCHAR(64) NOT NULL,
    `created_unix` integer,
    `updated_unix` integer,
    CONSTRAINT `uni_access_token_sha1` UNIQUE (`sha1`),
    CONSTRAINT `uni_access_token_sha256` UNIQUE (`sha256`)
);

CREATE INDEX `idx_access_token_user_id` ON `access_token`(`uid`);

CREATE TABLE `action` (
    `id` integer PRIMARY KEY AUTOINCREMENT,
    `user_id` integer,
    `op_type` integer,
    `act_user_id` integer,
    `act_user_name` text,
    `repo_id` integer,
    `repo_user_name` text,
    `repo_name` text,
    `ref_name` text,
    `is_private` numeric NOT NULL DEFAULT false,
    `content` text,
    `created_unix` integer
);

CREATE INDEX `idx_action_repo_id` ON `action`(`repo_id`);

CREATE INDEX `idx_action_user_id` ON `action`(`user_id`);

CREATE TABLE `attachment` (
    `id` INTEGER PRIMARY KEY AUTOINCREMENT NOT NULL,
    `uuid` UUID NULL,
    `issue_id` INTEGER NULL,
    `comment_id` INTEGER NULL,
    `release_id` INTEGER NULL,
    `name` TEXT NULL,
    `created_unix` INTEGER NULL
);

CREATE INDEX `IDX_attachment_issue_id` ON `attachment` (`issue_id`);

CREATE INDEX `IDX_attachment_release_id` ON `attachment` (`release_id`);

CREATE UNIQUE INDEX `UQE_attachment_uuid` ON `attachment` (`uuid`);

CREATE TABLE `collaboration` (
    `id` INTEGER PRIMARY KEY AUTOINCREMENT NOT NULL,
    `user_id` INTEGER NOT NULL,
    `repo_id` INTEGER NOT NULL,
    `mode` INTEGER DEFAULT 2 NOT NULL
);

CREATE INDEX `IDX_collaboration_repo_id` ON `collaboration` (`repo_id`);

CREATE INDEX `IDX_collaboration_user_id` ON `collaboration` (`user_id`);

CREATE UNIQUE INDEX `UQE_collaboration_s` ON `collaboration` (`user_id`,`repo_id`);

CREATE TABLE `comment` (
    `id` INTEGER PRIMARY KEY AUTOINCREMENT NOT NULL,
    `type` INTEGER NULL,
    `poster_id` INTEGER NULL,
    `issue_id` INTEGER NULL,
    `commit_id` INTEGER NULL,
    `line` INTEGER NULL,
    `content` TEXT NULL,
    `created_unix` INTEGER NULL,
    `updated_unix` INTEGER NULL,
    `commit_sha` TEXT NULL
);

CREATE INDEX `IDX_comment_issue_id` ON `comment` (`issue_id`);

CREATE TABLE `deploy_key` (
    `id` INTEGER PRIMARY KEY AUTOINCREMENT NOT NULL,
    `key_id` INTEGER NULL,
    `repo_id` INTEGER NULL,
    `name` TEXT NULL,
    `fingerprint` TEXT NULL,
    `created_unix` INTEGER NULL,
    `updated_unix` INTEGER NULL
);

CREATE INDEX `IDX_deploy_key_key_id` ON `deploy_key` (`key_id`);

CREATE INDEX `IDX_deploy_key_repo_id` ON `deploy_key` (`repo_id`);

CREATE UNIQUE INDEX `UQE_deploy_key_s` ON `deploy_key` (`key_id`,`repo_id`);

CREATE TABLE `email_address` (
    `id` integer PRIMARY KEY AUTOINCREMENT,
    `uid` integer NOT NULL,
    `email` text NOT NULL,
    `is_activated` numeric NOT NULL DEFAULT false
);

CREATE UNIQUE INDEX `email_address_user_email_unique` ON `email_address`(`uid`,`email`);

CREATE INDEX `idx_email_address_user_id` ON `email_address`(`uid`);

CREATE TABLE `follow` (
    `id` integer PRIMARY KEY AUTOINCREMENT,
    `user_id` integer NOT NULL,
    `follow_id` integer NOT NULL
);

CREATE UNIQUE INDEX `follow_user_follow_unique` ON `follow`(`user_id`,`follow_id`);

CREATE TABLE `hook_task` (
    `id` INTEGER PRIMARY KEY AUTOINCREMENT NOT NULL,
    `repo_id` INTEGER NULL,
    `hook_id` INTEGER NULL,
    `uuid` TEXT NULL,
    `type` INTEGER NULL,
    `url` TEXT NULL,
    `signature` TEXT NULL,
    `payload_content` TEXT NULL,
    `content_type` INTEGER NULL,
    `event_type` TEXT NULL,
    `is_ssl` INTEGER NULL,
    `is_delivered` INTEGER NULL,
    `delivered` INTEGER NULL,
    `is_succeed` INTEGER NULL,
    `request_content` TEXT NULL,
    `response_content` TEXT NULL
);

CREATE INDEX `IDX_hook_task_repo_id` ON `hook_task` (`repo_id`);

CREATE TABLE `issue` (
    `id` INTEGER PRIMARY KEY AUTOINCREMENT NOT NULL,
    `repo_id` INTEGER NULL,
    `index` INTEGER NULL,
    `poster_id` INTEGER NULL,
    `name` TEXT NULL,
    `content` TEXT NULL,
    `milestone_id` INTEGER NULL,
    `priority` INTEGER NULL,
    `assignee_id` INTEGER NULL,
    `is_closed` INTEGER NULL,
    `is_pull` INTEGER NULL,
    `num_comments` INTEGER NULL,
    `deadline_unix` INTEGER NULL,
    `created_unix` INTEGER NULL,
    `updated_unix` INTEGER NULL
);

CREATE INDEX `IDX_issue_repo_id` ON `issue` (`repo_id`);

CREATE UNIQUE INDEX `UQE_issue_repo_index` ON `issue` (`repo_id`,`index`);

CREATE TABLE `issue_label` (
    `id` INTEGER PRIMARY KEY AUTOINCREMENT NOT NULL,
    `issue_id` INTEGER NULL,
    `label_id` INTEGER NULL
);

CREATE UNIQUE INDEX `UQE_issue_label_s` ON `issue_label` (`issue_id`,`label_id`);

CREATE TABLE `issue_user` (
    `id` INTEGER PRIMARY KEY AUTOINCREMENT NOT NULL,
    `uid` INTEGER NULL,
    `issue_id` INTEGER NULL,
    `repo_id` INTEGER NULL,
    `milestone_id` INTEGER NULL,
    `is_read` INTEGER NULL,
    `is_assigned` INTEGER NULL,
    `is_mentioned` INTEGER NULL,
    `is_poster` INTEGER NULL,
    `is_closed` INTEGER NULL
);

CREATE INDEX `IDX_issue_user_repo_id` ON `issue_user` (`repo_id`);

CREATE INDEX `IDX_issue_user_uid` ON `issue_user` (`uid`);

CREATE TABLE `label` (
    `id` INTEGER PRIMARY KEY AUTOINCREMENT NOT NULL,
    `repo_id` INTEGER NULL,
    `name` TEXT NULL,
    `color` TEXT NULL,
    `num_issues` INTEGER NULL,
    `num_closed_issues` INTEGER NULL
);

CREATE INDEX `IDX_label_repo_id` ON `label` (`repo_id`);

CREATE TABLE `lfs_object` (
    `repo_id` integer,
    `oid` text,
    `size` integer NOT NULL,
    `storage` text NOT NULL,
    `created_at` datetime NOT NULL,
    PRIMARY KEY (`repo_id`,`oid`)
);

CREATE TABLE `login_source` (
    `id` integer PRIMARY KEY AUTOINCREMENT,
    `type` integer,
    `name` text,
    `is_actived` numeric NOT NULL,
    `is_default` numeric,
    `cfg` TEXT,
    `created_unix` integer,
    `updated_unix` integer,
    CONSTRAINT `uni_login_source_name` UNIQUE (`name`)
);

CREATE TABLE `milestone` (
    `id` INTEGER PRIMARY KEY AUTOINCREMENT NOT NULL,
    `repo_id` INTEGER NULL,
    `name` TEXT NULL,
    `content` TEXT NULL,
    `is_closed` INTEGER NULL,
    `num_issues` INTEGER NULL,
    `num_closed_issues` INTEGER NULL,
    `completeness` INTEGER NULL,
    `deadline_unix` INTEGER NULL,
    `closed_date_unix` INTEGER NULL
);

CREATE INDEX `IDX_milestone_repo_id` ON `milestone` (`repo_id`);

CREATE TABLE `mirror` (
    `id` INTEGER PRIMARY KEY AUTOINCREMENT NOT NULL,
    `repo_id` INTEGER NULL,
    `interval` INTEGER NULL,
    `enable_prune` INTEGER DEFAULT 1 NOT NULL,
    `updated_unix` INTEGER NULL,
    `next_update_unix` INTEGER NULL
);

CREATE TABLE `notice` (
    `id` integer PRIMARY KEY AUTOINCREMENT,
    `type` integer,
    `description` TEXT,
    `created_unix` integer
);

CREATE TABLE `org_user` (
    `id` INTEGER PRIMARY KEY AUTOINCREMENT NOT NULL,
    `uid` INTEGER NULL,
    `org_id` INTEGER NULL,
    `is_public` INTEGER NULL,
    `is_owner` INTEGER NULL,
    `num_teams` INTEGER NULL
);

CREATE INDEX `IDX_org_user_org_id` ON `org_user` (`org_id`);

CREATE INDEX `IDX_org_user_uid` ON `org_user` (`uid`);

CREATE UNIQUE INDEX `UQE_org_user_s` ON `org_user` (`uid`,`org_id`);

CREATE TABLE `protect_branch` (
    `id` INTEGER PRIMARY KEY AUTOINCREMENT NOT NULL,
    `repo_id` INTEGER NULL,
    `name` TEXT NULL,
    `protected` INTEGER NULL,
    `require_pull_request` INTEGER NULL,
    `enable_whitelist` INTEGER NULL,
    `whitelist_user_i_ds` TEXT NULL,
    `whitelist_team_i_ds` TEXT NULL
);

CREATE UNIQUE INDEX `UQE_protect_branch_protect_branch` ON `protect_branch` (`repo_id`,`name`);

CREATE TABLE `protect_branch_whitelist` (
    `id` INTEGER PRIMARY KEY AUTOINCREMENT NOT NULL,
    `protect_branch_id` INTEGER NULL,
    `repo_id` INTEGER NULL,
    `name` TEXT NULL,
    `user_id` INTEGER NULL
);

CREATE UNIQUE INDEX `UQE_protect_branch_whitelist_protect_branch_whitelist` ON `protect_branch_whitelist` (`repo_id`,`name`,`user_id`);

CREATE TABLE `public_key` (
    `id` INTEGER PRIMARY KEY AUTOINCREMENT NOT NULL,
    `owner_id` INTEGER NOT NULL,
    `name` TEXT NOT NULL,
    `fingerprint` TEXT NOT NULL,
    `content` TEXT NOT NULL,
    `mode` INTEGER DEFAULT 2 NOT NULL,
    `type` INTEGER DEFAULT 1 NOT NULL,
    `created_unix` INTEGER NULL,
    `updated_unix` INTEGER NULL
);

CREATE INDEX `IDX_public_key_owner_id` ON `public_key` (`owner_id`);

CREATE TABLE `pull_request` (
    `id` INTEGER PRIMARY KEY AUTOINCREMENT NOT NULL,
    `type` INTEGER NULL,
    `status` INTEGER NULL,
    `issue_id` INTEGER NULL,
    `index` INTEGER NULL,
    `head_repo_id` INTEGER NULL,
    `base_repo_id` INTEGER NULL,
    `head_user_name` TEXT NULL,
    `head_branch` TEXT NULL,
    `base_branch` TEXT NULL,
    `merge_base` TEXT NULL,
    `has_merged` INTEGER NULL,
    `merged_commit_id` TEXT NULL,
    `merger_id` INTEGER NULL,
    `merged_unix` INTEGER NULL
);

CREATE INDEX `IDX_pull_request_issue_id` ON `pull_request` (`issue_id`);

CREATE TABLE `release` (
    `id` INTEGER PRIMARY KEY AUTOINCREMENT NOT NULL,
    `repo_id` INTEGER NULL,
    `publisher_id` INTEGER NULL,
    `tag_name` TEXT NULL,
    `lower_tag_name` TEXT NULL,
    `target` TEXT NULL,
    `title` TEXT NULL,
    `sha1` TEXT NULL,
    `num_commits` INTEGER NULL,
    `note` TEXT NULL,
    `is_draft` INTEGER DEFAULT 0 NOT NULL,
    `is_prerelease` INTEGER NULL,
    `created_unix` INTEGER NULL
);

CREATE TABLE `repository` (
    `id` INTEGER PRIMARY KEY AUTOINCREMENT NOT NULL,
    `owner_id` INTEGER NULL,
    `lower_name` TEXT NOT NULL,
    `name` TEXT NOT NULL,
    `description` TEXT NULL,
    `website` TEXT NULL,
    `default_branch` TEXT NULL,
    `size` INTEGER DEFAULT 0 NOT NULL,
    `use_custom_avatar` INTEGER NULL,
    `num_watches` INTEGER NULL,
    `num_stars` INTEGER NULL,
    `num_forks` INTEGER NULL,
    `num_issues` INTEGER NULL,
    `num_closed_issues` INTEGER NULL,
    `num_pulls` INTEGER NULL,
    `num_closed_pulls` INTEGER NULL,
    `num_milestones` INTEGER DEFAULT 0 NOT NULL,
    `num_closed_milestones` INTEGER DEFAULT 0 NOT NULL,
    `is_private` INTEGER NULL,
    `is_unlisted` INTEGER DEFAULT 0 NOT NULL,
    `is_bare` INTEGER NULL,
    `is_mirror` INTEGER NULL,
    `enable_wiki` INTEGER DEFAULT 1 NOT NULL,
    `allow_public_wiki` INTEGER NULL,
    `enable_external_wiki` INTEGER NULL,
    `external_wiki_url` TEXT NULL,
    `enable_issues` INTEGER DEFAULT 1 NOT NULL,
    `allow_public_issues` INTEGER NULL,
    `enable_external_tracker` INTEGER NULL,
    `external_tracker_url` TEXT NULL,
    `external_tracker_format` TEXT NULL,
    `external_tracker_style` TEXT NULL,
    `enable_pulls` INTEGER DEFAULT 1 NOT NULL,
    `pulls_ignore_whitespace` INTEGER DEFAULT 0 NOT NULL,
    `pulls_allow_rebase` INTEGER DEFAULT 0 NOT NULL,
    `is_fork` INTEGER DEFAULT 0 NOT NULL,
    `fork_id` INTEGER NULL,
    `created_unix` INTEGER NULL,
    `updated_unix` INTEGER NULL
);

CREATE INDEX `IDX_repository_lower_name` ON `repository` (`lower_name`);

CREATE INDEX `IDX_repository_name` ON `repository` (`name`);

CREATE UNIQUE INDEX `UQE_repository_s` ON `repository` (`owner_id`,`lower_name`);

CREATE TABLE `star` (
    `id` INTEGER PRIMARY KEY AUTOINCREMENT NOT NULL,
    `uid` INTEGER NULL,
    `repo_id` INTEGER NULL
);

CREATE UNIQUE INDEX `UQE_star_s` ON `star` (`uid`,`repo_id`);

CREATE TABLE `team` (
    `id` INTEGER PRIMARY KEY AUTOINCREMENT NOT NULL,
    `org_id` INTEGER NULL,
    `lower_name` TEXT NULL,
    `name` TEXT NULL,
    `description` TEXT NULL,
    `authorize` INTEGER NULL,
    `num_repos` INTEGER NULL,
    `num_members` INTEGER NULL
);

CREATE INDEX `IDX_team_org_id` ON `team` (`org_id`);

CREATE TABLE `team_repo` (
    `id` INTEGER PRIMARY KEY AUTOINCREMENT NOT NULL,
    `org_id` INTEGER NULL,
    `team_id` INTEGER NULL,
    `repo_id` INTEGER NULL
);

CREATE INDEX `IDX_team_repo_org_id` ON `team_repo` (`org_id`);

CREATE UNIQUE INDEX `UQE_team_repo_s` ON `team_repo` (`team_id`,`repo_id`);

CREATE TABLE `team_user` (
    `id` INTEGER PRIMARY KEY AUTOINCREMENT NOT NULL,
    `org_id` INTEGER NULL,
    `team_id` INTEGER NULL,
    `uid` INTEGER NULL
);

CREATE INDEX `IDX_team_user_org_id` ON `team_user` (`org_id`);

CREATE UNIQUE INDEX `UQE_team_user_s` ON `team_user` (`team_id`,`uid`);

CREATE TABLE `two_factor` (
    `id` INTEGER PRIMARY KEY AUTOINCREMENT NOT NULL,
    `user_id` INTEGER NULL,
    `secret` TEXT NULL,
    `created_unix` INTEGER NULL
);

CREATE UNIQUE INDEX `UQE_two_factor_user_id` ON `two_factor` (`user_id`);

CREATE TABLE `two_factor_recovery_code` (
    `id` INTEGER PRIMARY KEY AUTOINCREMENT NOT NULL,
    `user_id` INTEGER NULL,
    `code` TEXT NULL,
    `is_used` INTEGER NULL
);

CREATE TABLE `upload` (
    `id` INTEGER PRIMARY KEY AUTOINCREMENT NOT NULL,
    `uuid` UUID NULL,
    `name` TEXT NULL
);

CREATE UNIQUE INDEX `UQE_upload_uuid` ON `upload` (`uuid`);

CREATE TABLE `user` (
    `id` INTEGER PRIMARY KEY AUTOINCREMENT NOT NULL,
    `lower_name` TEXT NOT NULL,
    `name` TEXT NOT NULL,
    `full_name` TEXT NULL,
    `email` TEXT NOT NULL,
    `passwd` TEXT NOT NULL,
    `login_source` INTEGER DEFAULT 0 NOT NULL,
    `login_name` TEXT NULL,
    `type` INTEGER NULL,
    `location` TEXT NULL,
    `website` TEXT NULL,
    `rands` TEXT NULL,
    `salt` TEXT NULL,
    `created_unix` INTEGER NULL,
    `updated_unix` INTEGER NULL,
    `last_repo_visibility` INTEGER NULL,
    `max_repo_creation` INTEGER DEFAULT -1 NOT NULL,
    `is_active` INTEGER NULL,
    `is_admin` INTEGER NULL,
    `allow_git_hook` INTEGER NULL,
    `allow_import_local` INTEGER NULL,
    `prohibit_login` INTEGER NULL,
    `avatar` TEXT NOT NULL,
    `avatar_email` TEXT NOT NULL,
    `use_custom_avatar` INTEGER NULL,
    `num_followers` INTEGER NULL,
    `num_following` INTEGER DEFAULT 0 NOT NULL,
    `num_stars` INTEGER NULL,
    `num_repos` INTEGER NULL,
    `description` TEXT NULL,
    `num_teams` INTEGER NULL,
    `num_members` INTEGER NULL
);

CREATE UNIQUE INDEX `UQE_user_lower_name` ON `user` (`lower_name`);

CREATE UNIQUE INDEX `UQE_user_name` ON `user` (`name`);

CREATE TABLE `watch` (
    `id` INTEGER PRIMARY KEY AUTOINCREMENT NOT NULL,
    `user_id` INTEGER NULL,
    `repo_id` INTEGER NULL
);

CREATE UNIQUE INDEX `UQE_watch_watch` ON `watch` (`user_id`,`repo_id`);

CREATE TABLE `webhook` (
    `id` INTEGER PRIMARY KEY AUTOINCREMENT NOT NULL,
    `repo_id` INTEGER NULL,
    `org_id` INTEGER NULL,
    `url` TEXT NULL,
    `content_type` INTEGER NULL,
    `secret` TEXT NULL,
    `events` TEXT NULL,
    `is_ssl` INTEGER NULL,
    `is_active` INTEGER NULL,
    `hook_task_type` INTEGER NULL,
    `meta` TEXT NULL,
    `last_status` INTEGER NULL,
    `created_unix` INTEGER NULL,
    `updated_unix` INTEGER NULL
);
