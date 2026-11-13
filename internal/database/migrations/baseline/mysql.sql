-- Tables as Gogs created them on a new installation at schema version 22. Change
-- the schema with new migrations, never by editing this file.

CREATE TABLE `access` (
  `id` bigint NOT NULL AUTO_INCREMENT,
  `user_id` bigint NOT NULL,
  `repo_id` bigint NOT NULL,
  `mode` bigint NOT NULL,
  PRIMARY KEY (`id`),
  UNIQUE KEY `access_user_repo_unique` (`user_id`,`repo_id`)
) ENGINE=InnoDB;

CREATE TABLE `access_token` (
  `id` bigint NOT NULL AUTO_INCREMENT,
  `uid` bigint DEFAULT NULL,
  `name` longtext DEFAULT NULL,
  `sha1` varchar(40) DEFAULT NULL,
  `sha256` varchar(64) NOT NULL,
  `created_unix` bigint DEFAULT NULL,
  `updated_unix` bigint DEFAULT NULL,
  PRIMARY KEY (`id`),
  UNIQUE KEY `uni_access_token_sha256` (`sha256`),
  UNIQUE KEY `uni_access_token_sha1` (`sha1`),
  KEY `idx_access_token_user_id` (`uid`)
) ENGINE=InnoDB;

CREATE TABLE `action` (
  `id` bigint NOT NULL AUTO_INCREMENT,
  `user_id` bigint DEFAULT NULL,
  `op_type` bigint DEFAULT NULL,
  `act_user_id` bigint DEFAULT NULL,
  `act_user_name` longtext DEFAULT NULL,
  `repo_id` bigint DEFAULT NULL,
  `repo_user_name` longtext DEFAULT NULL,
  `repo_name` longtext DEFAULT NULL,
  `ref_name` longtext DEFAULT NULL,
  `is_private` tinyint(1) NOT NULL DEFAULT 0,
  `content` longtext DEFAULT NULL,
  `created_unix` bigint DEFAULT NULL,
  PRIMARY KEY (`id`),
  KEY `idx_action_user_id` (`user_id`),
  KEY `idx_action_repo_id` (`repo_id`)
) ENGINE=InnoDB;

CREATE TABLE `attachment` (
  `id` bigint NOT NULL AUTO_INCREMENT,
  `uuid` varchar(40) DEFAULT NULL,
  `issue_id` bigint DEFAULT NULL,
  `comment_id` bigint DEFAULT NULL,
  `release_id` bigint DEFAULT NULL,
  `name` varchar(255) DEFAULT NULL,
  `created_unix` bigint DEFAULT NULL,
  PRIMARY KEY (`id`),
  UNIQUE KEY `UQE_attachment_uuid` (`uuid`),
  KEY `IDX_attachment_issue_id` (`issue_id`),
  KEY `IDX_attachment_release_id` (`release_id`)
) ENGINE=InnoDB ROW_FORMAT=DYNAMIC;

CREATE TABLE `collaboration` (
  `id` bigint NOT NULL AUTO_INCREMENT,
  `user_id` bigint NOT NULL,
  `repo_id` bigint NOT NULL,
  `mode` int NOT NULL DEFAULT 2,
  PRIMARY KEY (`id`),
  UNIQUE KEY `UQE_collaboration_s` (`user_id`,`repo_id`),
  KEY `IDX_collaboration_user_id` (`user_id`),
  KEY `IDX_collaboration_repo_id` (`repo_id`)
) ENGINE=InnoDB ROW_FORMAT=DYNAMIC;

CREATE TABLE `comment` (
  `id` bigint NOT NULL AUTO_INCREMENT,
  `type` int DEFAULT NULL,
  `poster_id` bigint DEFAULT NULL,
  `issue_id` bigint DEFAULT NULL,
  `commit_id` bigint DEFAULT NULL,
  `line` bigint DEFAULT NULL,
  `content` text DEFAULT NULL,
  `created_unix` bigint DEFAULT NULL,
  `updated_unix` bigint DEFAULT NULL,
  `commit_sha` varchar(40) DEFAULT NULL,
  PRIMARY KEY (`id`),
  KEY `IDX_comment_issue_id` (`issue_id`)
) ENGINE=InnoDB ROW_FORMAT=DYNAMIC;

CREATE TABLE `deploy_key` (
  `id` bigint NOT NULL AUTO_INCREMENT,
  `key_id` bigint DEFAULT NULL,
  `repo_id` bigint DEFAULT NULL,
  `name` varchar(255) DEFAULT NULL,
  `fingerprint` varchar(255) DEFAULT NULL,
  `created_unix` bigint DEFAULT NULL,
  `updated_unix` bigint DEFAULT NULL,
  PRIMARY KEY (`id`),
  UNIQUE KEY `UQE_deploy_key_s` (`key_id`,`repo_id`),
  KEY `IDX_deploy_key_key_id` (`key_id`),
  KEY `IDX_deploy_key_repo_id` (`repo_id`)
) ENGINE=InnoDB ROW_FORMAT=DYNAMIC;

CREATE TABLE `email_address` (
  `id` bigint NOT NULL AUTO_INCREMENT,
  `uid` bigint NOT NULL,
  `email` varchar(254) NOT NULL,
  `is_activated` tinyint(1) NOT NULL DEFAULT 0,
  PRIMARY KEY (`id`),
  UNIQUE KEY `email_address_user_email_unique` (`uid`,`email`),
  KEY `idx_email_address_user_id` (`uid`)
) ENGINE=InnoDB;

CREATE TABLE `follow` (
  `id` bigint NOT NULL AUTO_INCREMENT,
  `user_id` bigint NOT NULL,
  `follow_id` bigint NOT NULL,
  PRIMARY KEY (`id`),
  UNIQUE KEY `follow_user_follow_unique` (`user_id`,`follow_id`)
) ENGINE=InnoDB;

CREATE TABLE `hook_task` (
  `id` bigint NOT NULL AUTO_INCREMENT,
  `repo_id` bigint DEFAULT NULL,
  `hook_id` bigint DEFAULT NULL,
  `uuid` varchar(255) DEFAULT NULL,
  `type` int DEFAULT NULL,
  `url` text DEFAULT NULL,
  `signature` text DEFAULT NULL,
  `payload_content` text DEFAULT NULL,
  `content_type` int DEFAULT NULL,
  `event_type` varchar(255) DEFAULT NULL,
  `is_ssl` tinyint(1) DEFAULT NULL,
  `is_delivered` tinyint(1) DEFAULT NULL,
  `delivered` bigint DEFAULT NULL,
  `is_succeed` tinyint(1) DEFAULT NULL,
  `request_content` text DEFAULT NULL,
  `response_content` text DEFAULT NULL,
  PRIMARY KEY (`id`),
  KEY `IDX_hook_task_repo_id` (`repo_id`)
) ENGINE=InnoDB ROW_FORMAT=DYNAMIC;

CREATE TABLE `issue` (
  `id` bigint NOT NULL AUTO_INCREMENT,
  `repo_id` bigint DEFAULT NULL,
  `index` bigint DEFAULT NULL,
  `poster_id` bigint DEFAULT NULL,
  `name` varchar(255) DEFAULT NULL,
  `content` text DEFAULT NULL,
  `milestone_id` bigint DEFAULT NULL,
  `priority` int DEFAULT NULL,
  `assignee_id` bigint DEFAULT NULL,
  `is_closed` tinyint(1) DEFAULT NULL,
  `is_pull` tinyint(1) DEFAULT NULL,
  `num_comments` int DEFAULT NULL,
  `deadline_unix` bigint DEFAULT NULL,
  `created_unix` bigint DEFAULT NULL,
  `updated_unix` bigint DEFAULT NULL,
  PRIMARY KEY (`id`),
  UNIQUE KEY `UQE_issue_repo_index` (`repo_id`,`index`),
  KEY `IDX_issue_repo_id` (`repo_id`)
) ENGINE=InnoDB ROW_FORMAT=DYNAMIC;

CREATE TABLE `issue_label` (
  `id` bigint NOT NULL AUTO_INCREMENT,
  `issue_id` bigint DEFAULT NULL,
  `label_id` bigint DEFAULT NULL,
  PRIMARY KEY (`id`),
  UNIQUE KEY `UQE_issue_label_s` (`issue_id`,`label_id`)
) ENGINE=InnoDB ROW_FORMAT=DYNAMIC;

CREATE TABLE `issue_user` (
  `id` bigint NOT NULL AUTO_INCREMENT,
  `uid` bigint DEFAULT NULL,
  `issue_id` bigint DEFAULT NULL,
  `repo_id` bigint DEFAULT NULL,
  `milestone_id` bigint DEFAULT NULL,
  `is_read` tinyint(1) DEFAULT NULL,
  `is_assigned` tinyint(1) DEFAULT NULL,
  `is_mentioned` tinyint(1) DEFAULT NULL,
  `is_poster` tinyint(1) DEFAULT NULL,
  `is_closed` tinyint(1) DEFAULT NULL,
  PRIMARY KEY (`id`),
  KEY `IDX_issue_user_uid` (`uid`),
  KEY `IDX_issue_user_repo_id` (`repo_id`)
) ENGINE=InnoDB ROW_FORMAT=DYNAMIC;

CREATE TABLE `label` (
  `id` bigint NOT NULL AUTO_INCREMENT,
  `repo_id` bigint DEFAULT NULL,
  `name` varchar(255) DEFAULT NULL,
  `color` varchar(7) DEFAULT NULL,
  `num_issues` int DEFAULT NULL,
  `num_closed_issues` int DEFAULT NULL,
  PRIMARY KEY (`id`),
  KEY `IDX_label_repo_id` (`repo_id`)
) ENGINE=InnoDB ROW_FORMAT=DYNAMIC;

CREATE TABLE `lfs_object` (
  `repo_id` bigint NOT NULL,
  `oid` varchar(191) NOT NULL,
  `size` bigint NOT NULL,
  `storage` longtext NOT NULL,
  `created_at` datetime(3) NOT NULL,
  PRIMARY KEY (`repo_id`,`oid`)
) ENGINE=InnoDB;

CREATE TABLE `login_source` (
  `id` bigint NOT NULL AUTO_INCREMENT,
  `type` bigint DEFAULT NULL,
  `name` varchar(191) DEFAULT NULL,
  `is_actived` tinyint(1) NOT NULL,
  `is_default` tinyint(1) DEFAULT NULL,
  `cfg` text DEFAULT NULL,
  `created_unix` bigint DEFAULT NULL,
  `updated_unix` bigint DEFAULT NULL,
  PRIMARY KEY (`id`),
  UNIQUE KEY `uni_login_source_name` (`name`)
) ENGINE=InnoDB;

CREATE TABLE `milestone` (
  `id` bigint NOT NULL AUTO_INCREMENT,
  `repo_id` bigint DEFAULT NULL,
  `name` varchar(255) DEFAULT NULL,
  `content` text DEFAULT NULL,
  `is_closed` tinyint(1) DEFAULT NULL,
  `num_issues` int DEFAULT NULL,
  `num_closed_issues` int DEFAULT NULL,
  `completeness` int DEFAULT NULL,
  `deadline_unix` bigint DEFAULT NULL,
  `closed_date_unix` bigint DEFAULT NULL,
  PRIMARY KEY (`id`),
  KEY `IDX_milestone_repo_id` (`repo_id`)
) ENGINE=InnoDB ROW_FORMAT=DYNAMIC;

CREATE TABLE `mirror` (
  `id` bigint NOT NULL AUTO_INCREMENT,
  `repo_id` bigint DEFAULT NULL,
  `interval` int DEFAULT NULL,
  `enable_prune` tinyint(1) NOT NULL DEFAULT 1,
  `updated_unix` bigint DEFAULT NULL,
  `next_update_unix` bigint DEFAULT NULL,
  PRIMARY KEY (`id`)
) ENGINE=InnoDB ROW_FORMAT=DYNAMIC;

CREATE TABLE `notice` (
  `id` bigint NOT NULL AUTO_INCREMENT,
  `type` bigint DEFAULT NULL,
  `description` text DEFAULT NULL,
  `created_unix` bigint DEFAULT NULL,
  PRIMARY KEY (`id`)
) ENGINE=InnoDB;

CREATE TABLE `org_user` (
  `id` bigint NOT NULL AUTO_INCREMENT,
  `uid` bigint DEFAULT NULL,
  `org_id` bigint DEFAULT NULL,
  `is_public` tinyint(1) DEFAULT NULL,
  `is_owner` tinyint(1) DEFAULT NULL,
  `num_teams` int DEFAULT NULL,
  PRIMARY KEY (`id`),
  UNIQUE KEY `UQE_org_user_s` (`uid`,`org_id`),
  KEY `IDX_org_user_uid` (`uid`),
  KEY `IDX_org_user_org_id` (`org_id`)
) ENGINE=InnoDB ROW_FORMAT=DYNAMIC;

CREATE TABLE `protect_branch` (
  `id` bigint NOT NULL AUTO_INCREMENT,
  `repo_id` bigint DEFAULT NULL,
  `name` varchar(255) DEFAULT NULL,
  `protected` tinyint(1) DEFAULT NULL,
  `require_pull_request` tinyint(1) DEFAULT NULL,
  `enable_whitelist` tinyint(1) DEFAULT NULL,
  `whitelist_user_i_ds` text DEFAULT NULL,
  `whitelist_team_i_ds` text DEFAULT NULL,
  PRIMARY KEY (`id`),
  UNIQUE KEY `UQE_protect_branch_protect_branch` (`repo_id`,`name`)
) ENGINE=InnoDB ROW_FORMAT=DYNAMIC;

CREATE TABLE `protect_branch_whitelist` (
  `id` bigint NOT NULL AUTO_INCREMENT,
  `protect_branch_id` bigint DEFAULT NULL,
  `repo_id` bigint DEFAULT NULL,
  `name` varchar(255) DEFAULT NULL,
  `user_id` bigint DEFAULT NULL,
  PRIMARY KEY (`id`),
  UNIQUE KEY `UQE_protect_branch_whitelist_protect_branch_whitelist` (`repo_id`,`name`,`user_id`)
) ENGINE=InnoDB ROW_FORMAT=DYNAMIC;

CREATE TABLE `public_key` (
  `id` bigint NOT NULL AUTO_INCREMENT,
  `owner_id` bigint NOT NULL,
  `name` varchar(255) NOT NULL,
  `fingerprint` varchar(255) NOT NULL,
  `content` text NOT NULL,
  `mode` int NOT NULL DEFAULT 2,
  `type` int NOT NULL DEFAULT 1,
  `created_unix` bigint DEFAULT NULL,
  `updated_unix` bigint DEFAULT NULL,
  PRIMARY KEY (`id`),
  KEY `IDX_public_key_owner_id` (`owner_id`)
) ENGINE=InnoDB ROW_FORMAT=DYNAMIC;

CREATE TABLE `pull_request` (
  `id` bigint NOT NULL AUTO_INCREMENT,
  `type` int DEFAULT NULL,
  `status` int DEFAULT NULL,
  `issue_id` bigint DEFAULT NULL,
  `index` bigint DEFAULT NULL,
  `head_repo_id` bigint DEFAULT NULL,
  `base_repo_id` bigint DEFAULT NULL,
  `head_user_name` varchar(255) DEFAULT NULL,
  `head_branch` varchar(255) DEFAULT NULL,
  `base_branch` varchar(255) DEFAULT NULL,
  `merge_base` varchar(40) DEFAULT NULL,
  `has_merged` tinyint(1) DEFAULT NULL,
  `merged_commit_id` varchar(40) DEFAULT NULL,
  `merger_id` bigint DEFAULT NULL,
  `merged_unix` bigint DEFAULT NULL,
  PRIMARY KEY (`id`),
  KEY `IDX_pull_request_issue_id` (`issue_id`)
) ENGINE=InnoDB ROW_FORMAT=DYNAMIC;

CREATE TABLE `release` (
  `id` bigint NOT NULL AUTO_INCREMENT,
  `repo_id` bigint DEFAULT NULL,
  `publisher_id` bigint DEFAULT NULL,
  `tag_name` varchar(255) DEFAULT NULL,
  `lower_tag_name` varchar(255) DEFAULT NULL,
  `target` varchar(255) DEFAULT NULL,
  `title` varchar(255) DEFAULT NULL,
  `sha1` varchar(40) DEFAULT NULL,
  `num_commits` bigint DEFAULT NULL,
  `note` text DEFAULT NULL,
  `is_draft` tinyint(1) NOT NULL DEFAULT 0,
  `is_prerelease` tinyint(1) DEFAULT NULL,
  `created_unix` bigint DEFAULT NULL,
  PRIMARY KEY (`id`)
) ENGINE=InnoDB ROW_FORMAT=DYNAMIC;

CREATE TABLE `repository` (
  `id` bigint NOT NULL AUTO_INCREMENT,
  `owner_id` bigint DEFAULT NULL,
  `lower_name` varchar(255) NOT NULL,
  `name` varchar(255) NOT NULL,
  `description` varchar(512) DEFAULT NULL,
  `website` varchar(255) DEFAULT NULL,
  `default_branch` varchar(255) DEFAULT NULL,
  `size` bigint NOT NULL DEFAULT 0,
  `use_custom_avatar` tinyint(1) DEFAULT NULL,
  `num_watches` int DEFAULT NULL,
  `num_stars` int DEFAULT NULL,
  `num_forks` int DEFAULT NULL,
  `num_issues` int DEFAULT NULL,
  `num_closed_issues` int DEFAULT NULL,
  `num_pulls` int DEFAULT NULL,
  `num_closed_pulls` int DEFAULT NULL,
  `num_milestones` int NOT NULL DEFAULT 0,
  `num_closed_milestones` int NOT NULL DEFAULT 0,
  `is_private` tinyint(1) DEFAULT NULL,
  `is_unlisted` tinyint(1) NOT NULL DEFAULT 0,
  `is_bare` tinyint(1) DEFAULT NULL,
  `is_mirror` tinyint(1) DEFAULT NULL,
  `enable_wiki` tinyint(1) NOT NULL DEFAULT 1,
  `allow_public_wiki` tinyint(1) DEFAULT NULL,
  `enable_external_wiki` tinyint(1) DEFAULT NULL,
  `external_wiki_url` varchar(255) DEFAULT NULL,
  `enable_issues` tinyint(1) NOT NULL DEFAULT 1,
  `allow_public_issues` tinyint(1) DEFAULT NULL,
  `enable_external_tracker` tinyint(1) DEFAULT NULL,
  `external_tracker_url` varchar(255) DEFAULT NULL,
  `external_tracker_format` varchar(255) DEFAULT NULL,
  `external_tracker_style` varchar(255) DEFAULT NULL,
  `enable_pulls` tinyint(1) NOT NULL DEFAULT 1,
  `pulls_ignore_whitespace` tinyint(1) NOT NULL DEFAULT 0,
  `pulls_allow_rebase` tinyint(1) NOT NULL DEFAULT 0,
  `is_fork` tinyint(1) NOT NULL DEFAULT 0,
  `fork_id` bigint DEFAULT NULL,
  `created_unix` bigint DEFAULT NULL,
  `updated_unix` bigint DEFAULT NULL,
  PRIMARY KEY (`id`),
  UNIQUE KEY `UQE_repository_s` (`owner_id`,`lower_name`),
  KEY `IDX_repository_lower_name` (`lower_name`),
  KEY `IDX_repository_name` (`name`)
) ENGINE=InnoDB ROW_FORMAT=DYNAMIC;

CREATE TABLE `star` (
  `id` bigint NOT NULL AUTO_INCREMENT,
  `uid` bigint DEFAULT NULL,
  `repo_id` bigint DEFAULT NULL,
  PRIMARY KEY (`id`),
  UNIQUE KEY `UQE_star_s` (`uid`,`repo_id`)
) ENGINE=InnoDB ROW_FORMAT=DYNAMIC;

CREATE TABLE `team` (
  `id` bigint NOT NULL AUTO_INCREMENT,
  `org_id` bigint DEFAULT NULL,
  `lower_name` varchar(255) DEFAULT NULL,
  `name` varchar(255) DEFAULT NULL,
  `description` varchar(255) DEFAULT NULL,
  `authorize` int DEFAULT NULL,
  `num_repos` int DEFAULT NULL,
  `num_members` int DEFAULT NULL,
  PRIMARY KEY (`id`),
  KEY `IDX_team_org_id` (`org_id`)
) ENGINE=InnoDB ROW_FORMAT=DYNAMIC;

CREATE TABLE `team_repo` (
  `id` bigint NOT NULL AUTO_INCREMENT,
  `org_id` bigint DEFAULT NULL,
  `team_id` bigint DEFAULT NULL,
  `repo_id` bigint DEFAULT NULL,
  PRIMARY KEY (`id`),
  UNIQUE KEY `UQE_team_repo_s` (`team_id`,`repo_id`),
  KEY `IDX_team_repo_org_id` (`org_id`)
) ENGINE=InnoDB ROW_FORMAT=DYNAMIC;

CREATE TABLE `team_user` (
  `id` bigint NOT NULL AUTO_INCREMENT,
  `org_id` bigint DEFAULT NULL,
  `team_id` bigint DEFAULT NULL,
  `uid` bigint DEFAULT NULL,
  PRIMARY KEY (`id`),
  UNIQUE KEY `UQE_team_user_s` (`team_id`,`uid`),
  KEY `IDX_team_user_org_id` (`org_id`)
) ENGINE=InnoDB ROW_FORMAT=DYNAMIC;

CREATE TABLE `two_factor` (
  `id` bigint NOT NULL AUTO_INCREMENT,
  `user_id` bigint DEFAULT NULL,
  `secret` varchar(255) DEFAULT NULL,
  `created_unix` bigint DEFAULT NULL,
  PRIMARY KEY (`id`),
  UNIQUE KEY `UQE_two_factor_user_id` (`user_id`)
) ENGINE=InnoDB ROW_FORMAT=DYNAMIC;

CREATE TABLE `two_factor_recovery_code` (
  `id` bigint NOT NULL AUTO_INCREMENT,
  `user_id` bigint DEFAULT NULL,
  `code` varchar(11) DEFAULT NULL,
  `is_used` tinyint(1) DEFAULT NULL,
  PRIMARY KEY (`id`)
) ENGINE=InnoDB ROW_FORMAT=DYNAMIC;

CREATE TABLE `upload` (
  `id` bigint NOT NULL AUTO_INCREMENT,
  `uuid` varchar(40) DEFAULT NULL,
  `name` varchar(255) DEFAULT NULL,
  PRIMARY KEY (`id`),
  UNIQUE KEY `UQE_upload_uuid` (`uuid`)
) ENGINE=InnoDB ROW_FORMAT=DYNAMIC;

CREATE TABLE `user` (
  `id` bigint NOT NULL AUTO_INCREMENT,
  `lower_name` varchar(255) NOT NULL,
  `name` varchar(255) NOT NULL,
  `full_name` varchar(255) DEFAULT NULL,
  `email` varchar(255) NOT NULL,
  `passwd` varchar(255) NOT NULL,
  `login_source` bigint NOT NULL DEFAULT 0,
  `login_name` varchar(255) DEFAULT NULL,
  `type` int DEFAULT NULL,
  `location` varchar(255) DEFAULT NULL,
  `website` varchar(255) DEFAULT NULL,
  `rands` varchar(10) DEFAULT NULL,
  `salt` varchar(10) DEFAULT NULL,
  `created_unix` bigint DEFAULT NULL,
  `updated_unix` bigint DEFAULT NULL,
  `last_repo_visibility` tinyint(1) DEFAULT NULL,
  `max_repo_creation` int NOT NULL DEFAULT -1,
  `is_active` tinyint(1) DEFAULT NULL,
  `is_admin` tinyint(1) DEFAULT NULL,
  `allow_git_hook` tinyint(1) DEFAULT NULL,
  `allow_import_local` tinyint(1) DEFAULT NULL,
  `prohibit_login` tinyint(1) DEFAULT NULL,
  `avatar` varchar(2048) NOT NULL,
  `avatar_email` varchar(255) NOT NULL,
  `use_custom_avatar` tinyint(1) DEFAULT NULL,
  `num_followers` int DEFAULT NULL,
  `num_following` int NOT NULL DEFAULT 0,
  `num_stars` int DEFAULT NULL,
  `num_repos` int DEFAULT NULL,
  `description` varchar(255) DEFAULT NULL,
  `num_teams` int DEFAULT NULL,
  `num_members` int DEFAULT NULL,
  PRIMARY KEY (`id`),
  UNIQUE KEY `UQE_user_lower_name` (`lower_name`),
  UNIQUE KEY `UQE_user_name` (`name`)
) ENGINE=InnoDB ROW_FORMAT=DYNAMIC;

CREATE TABLE `watch` (
  `id` bigint NOT NULL AUTO_INCREMENT,
  `user_id` bigint DEFAULT NULL,
  `repo_id` bigint DEFAULT NULL,
  PRIMARY KEY (`id`),
  UNIQUE KEY `UQE_watch_watch` (`user_id`,`repo_id`)
) ENGINE=InnoDB ROW_FORMAT=DYNAMIC;

CREATE TABLE `webhook` (
  `id` bigint NOT NULL AUTO_INCREMENT,
  `repo_id` bigint DEFAULT NULL,
  `org_id` bigint DEFAULT NULL,
  `url` text DEFAULT NULL,
  `content_type` int DEFAULT NULL,
  `secret` text DEFAULT NULL,
  `events` text DEFAULT NULL,
  `is_ssl` tinyint(1) DEFAULT NULL,
  `is_active` tinyint(1) DEFAULT NULL,
  `hook_task_type` int DEFAULT NULL,
  `meta` text DEFAULT NULL,
  `last_status` int DEFAULT NULL,
  `created_unix` bigint DEFAULT NULL,
  `updated_unix` bigint DEFAULT NULL,
  PRIMARY KEY (`id`)
) ENGINE=InnoDB ROW_FORMAT=DYNAMIC;
