-- +goose Up

CREATE TABLE `gebra_store` (
    `id` text,
    `name` text NOT NULL,
    `model_id` text NOT NULL,
    `revision` integer NOT NULL,
    `created_at` datetime NOT NULL,
    `updated_at` datetime NOT NULL,
    `deleted_at` datetime,
    PRIMARY KEY (`id`)
);

CREATE INDEX `gebra_store_deleted_at` ON `gebra_store`(`deleted_at`);

CREATE TABLE `gebra_model` (
    `id` text,
    `store_id` text NOT NULL,
    `definition` text NOT NULL,
    `created_at` datetime NOT NULL,
    PRIMARY KEY (`id`)
);

CREATE INDEX `gebra_model_store` ON `gebra_model`(`store_id`);

CREATE TABLE `gebra_assertion` (
    `store_id` text,
    `model_id` text,
    `definition` text NOT NULL,
    PRIMARY KEY (`store_id`,`model_id`)
);

CREATE TABLE `gebra_tuple` (
    `id` integer PRIMARY KEY AUTOINCREMENT,
    `store_id` text NOT NULL,
    `object_type` text NOT NULL,
    `object_id` text NOT NULL,
    `relation` text NOT NULL,
    `user_type` text NOT NULL,
    `user_id` text NOT NULL,
    `user_relation` text NOT NULL,
    `condition_name` text NOT NULL,
    `condition_context` text NOT NULL,
    `created_at` datetime NOT NULL
);

CREATE INDEX `gebra_tuple_user` ON `gebra_tuple`(`store_id`,`user_type`,`user_id`,`user_relation`,`object_type`,`relation`);

CREATE UNIQUE INDEX `gebra_tuple_key` ON `gebra_tuple`(`store_id`,`object_type`,`object_id`,`relation`,`user_type`,`user_id`,`user_relation`);

CREATE TABLE `gebra_change` (
    `id` integer PRIMARY KEY AUTOINCREMENT,
    `store_id` text NOT NULL,
    `operation` text NOT NULL,
    `object_type` text NOT NULL,
    `object_id` text NOT NULL,
    `relation` text NOT NULL,
    `user_type` text NOT NULL,
    `user_id` text NOT NULL,
    `user_relation` text NOT NULL,
    `condition_name` text NOT NULL,
    `condition_context` text NOT NULL,
    `created_at` datetime NOT NULL
);

CREATE INDEX `gebra_change_store_id` ON `gebra_change`(`store_id`,`id`);

CREATE TABLE `gebra_object_attribute` (
    `store_id` text,
    `object_type` text,
    `name` text,
    `object_id` text,
    `value` text NOT NULL,
    PRIMARY KEY (`store_id`,`object_type`,`name`,`object_id`)
);

CREATE INDEX `gebra_object_attribute_sort` ON `gebra_object_attribute`(`store_id`,`object_type`,`name`,`value`,`object_id`);

CREATE TABLE `gebra_inbox` (
    `id` integer PRIMARY KEY AUTOINCREMENT,
    `store_id` text NOT NULL,
    `kind` text NOT NULL,
    `changes` text NOT NULL,
    `tuples` integer NOT NULL,
    `created_at` datetime NOT NULL
);

CREATE INDEX `gebra_inbox_store_id` ON `gebra_inbox`(`store_id`,`id`);

CREATE TABLE `gebra_pending` (
    `inbox_id` integer,
    `seq` integer,
    `store_id` text NOT NULL,
    `object_type` text NOT NULL,
    `object_id` text NOT NULL,
    `relation` text NOT NULL,
    `user_type` text NOT NULL,
    `user_id` text NOT NULL,
    `user_relation` text NOT NULL,
    `deleted` numeric NOT NULL,
    `condition_name` text NOT NULL,
    `condition_context` text NOT NULL,
    `created_at` datetime NOT NULL,
    PRIMARY KEY (`inbox_id`,`seq`)
);

CREATE INDEX `gebra_pending_key` ON `gebra_pending`(`store_id`,`object_type`,`object_id`,`relation`,`user_type`,`user_id`,`user_relation`,`inbox_id`);

CREATE TABLE `gebra_view_member` (
    `store_id` text,
    `gen` integer,
    `set_key` text,
    `user_type` text,
    `user_id` text,
    `set_type` text NOT NULL,
    `set_rel` text NOT NULL,
    `set_id` text NOT NULL,
    PRIMARY KEY (`store_id`,`gen`,`set_key`,`user_type`,`user_id`)
);

CREATE INDEX `gebra_view_member_user_set` ON `gebra_view_member`(`store_id`,`gen`,`user_type`,`user_id`,`set_type`,`set_rel`,`set_id`,`set_key`);

CREATE TABLE `gebra_view_edge` (
    `store_id` text,
    `gen` integer,
    `child_key` text,
    `parent_key` text,
    `refs` integer NOT NULL,
    PRIMARY KEY (`store_id`,`gen`,`child_key`,`parent_key`)
);

CREATE TABLE `gebra_view_reach` (
    `store_id` text,
    `gen` integer,
    `desc_key` text,
    `anc_type` text,
    `anc_rel` text,
    `anc_id` text,
    `desc_type` text NOT NULL,
    `desc_rel` text NOT NULL,
    `desc_id` text NOT NULL,
    PRIMARY KEY (`store_id`,`gen`,`desc_key`,`anc_type`,`anc_rel`,`anc_id`)
);

CREATE INDEX `gebra_view_reach_anc` ON `gebra_view_reach`(`store_id`,`gen`,`anc_type`,`anc_id`,`anc_rel`,`desc_type`,`desc_rel`);

CREATE TABLE `gebra_view_state` (
    `store_id` text,
    `applied` integer NOT NULL,
    `gen` integer NOT NULL DEFAULT 0,
    `model_id` text NOT NULL DEFAULT "",
    `member_rows` integer NOT NULL DEFAULT 0,
    `edge_rows` integer NOT NULL DEFAULT 0,
    `reach_rows` integer NOT NULL DEFAULT 0,
    `last_gen` integer NOT NULL DEFAULT 0,
    `build_gen` integer NOT NULL DEFAULT 0,
    `build_model_id` text NOT NULL DEFAULT "",
    `build_phase` text NOT NULL DEFAULT "",
    `build_object_type` text NOT NULL DEFAULT "",
    `build_object_id` text NOT NULL DEFAULT "",
    `build_relation` text NOT NULL DEFAULT "",
    `build_user_type` text NOT NULL DEFAULT "",
    `build_user_id` text NOT NULL DEFAULT "",
    `build_user_relation` text NOT NULL DEFAULT "",
    `build_set_key` text NOT NULL DEFAULT "",
    `build_done` integer NOT NULL DEFAULT 0,
    `build_total` integer NOT NULL DEFAULT 0,
    `build_chunks` integer NOT NULL DEFAULT 0,
    `build_member_rows` integer NOT NULL DEFAULT 0,
    `build_edge_rows` integer NOT NULL DEFAULT 0,
    `build_reach_rows` integer NOT NULL DEFAULT 0,
    `build_started_at` datetime,
    `updated_at` datetime NOT NULL,
    PRIMARY KEY (`store_id`)
);

CREATE TABLE `gebra_view_gc` (
    `store_id` text,
    `gen` integer,
    `delete_after` datetime NOT NULL,
    PRIMARY KEY (`store_id`,`gen`)
);

CREATE TABLE `gebra_change_cutoff` (
    `store_id` text,
    `change_id` integer NOT NULL,
    `updated_at` datetime NOT NULL,
    PRIMARY KEY (`store_id`)
);

CREATE TABLE `gebra_model_effect` (
    `model_id` text,
    `set_type` text,
    `set_rel` text,
    `tuple_rel` text,
    `kind` text,
    PRIMARY KEY (`model_id`,`set_type`,`set_rel`,`tuple_rel`,`kind`)
);
