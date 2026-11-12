# Table "access"

```
 Field  | Column  |   PostgreSQL    |         MySQL         |        SQLite3        
--------+---------+-----------------+-----------------------+-----------------------
 ID     | id      | BIGSERIAL       | BIGINT AUTO_INCREMENT | INTEGER AUTOINCREMENT 
 UserID | user_id | BIGINT NOT NULL | BIGINT NOT NULL       | INTEGER NOT NULL      
 RepoID | repo_id | BIGINT NOT NULL | BIGINT NOT NULL       | INTEGER NOT NULL      
 Mode   | mode    | BIGINT NOT NULL | BIGINT NOT NULL       | INTEGER NOT NULL      

Primary keys: id
Indexes: 
	"access_user_repo_unique" UNIQUE (user_id, repo_id)
```

# Table "access_token"

```
    Field    |    Column    |         PostgreSQL          |            MySQL            |           SQLite3           
-------------+--------------+-----------------------------+-----------------------------+-----------------------------
 ID          | id           | BIGSERIAL                   | BIGINT AUTO_INCREMENT       | INTEGER AUTOINCREMENT       
 UserID      | uid          | BIGINT                      | BIGINT                      | INTEGER                     
 Name        | name         | TEXT                        | LONGTEXT                    | TEXT                        
 Sha1        | sha1         | VARCHAR(40) UNIQUE          | VARCHAR(40) UNIQUE          | VARCHAR(40) UNIQUE          
 SHA256      | sha256       | VARCHAR(64) NOT NULL UNIQUE | VARCHAR(64) NOT NULL UNIQUE | VARCHAR(64) NOT NULL UNIQUE 
 CreatedUnix | created_unix | BIGINT                      | BIGINT                      | INTEGER                     
 UpdatedUnix | updated_unix | BIGINT                      | BIGINT                      | INTEGER                     

Primary keys: id
Indexes: 
	"idx_access_token_user_id" (uid)
```

# Table "action"

```
    Field     |     Column     |           PostgreSQL           |             MySQL              |            SQLite3             
--------------+----------------+--------------------------------+--------------------------------+--------------------------------
 ID           | id             | BIGSERIAL                      | BIGINT AUTO_INCREMENT          | INTEGER AUTOINCREMENT          
 UserID       | user_id        | BIGINT                         | BIGINT                         | INTEGER                        
 OpType       | op_type        | BIGINT                         | BIGINT                         | INTEGER                        
 ActUserID    | act_user_id    | BIGINT                         | BIGINT                         | INTEGER                        
 ActUserName  | act_user_name  | TEXT                           | LONGTEXT                       | TEXT                           
 RepoID       | repo_id        | BIGINT                         | BIGINT                         | INTEGER                        
 RepoUserName | repo_user_name | TEXT                           | LONGTEXT                       | TEXT                           
 RepoName     | repo_name      | TEXT                           | LONGTEXT                       | TEXT                           
 RefName      | ref_name       | TEXT                           | LONGTEXT                       | TEXT                           
 IsPrivate    | is_private     | BOOLEAN NOT NULL DEFAULT FALSE | BOOLEAN NOT NULL DEFAULT FALSE | NUMERIC NOT NULL DEFAULT FALSE 
 Content      | content        | TEXT                           | LONGTEXT                       | TEXT                           
 CreatedUnix  | created_unix   | BIGINT                         | BIGINT                         | INTEGER                        

Primary keys: id
Indexes: 
	"idx_action_repo_id" (repo_id)
	"idx_action_user_id" (user_id)
```

# Table "attachment"

```
    Field    |    Column    |  PostgreSQL  |         MySQL         |        SQLite3        
-------------+--------------+--------------+-----------------------+-----------------------
 ID          | id           | BIGSERIAL    | BIGINT AUTO_INCREMENT | INTEGER AUTOINCREMENT 
 UUID        | uuid         | VARCHAR(40)  | VARCHAR(40)           | TEXT                  
 IssueID     | issue_id     | BIGINT       | BIGINT                | INTEGER               
 CommentID   | comment_id   | BIGINT       | BIGINT                | INTEGER               
 ReleaseID   | release_id   | BIGINT       | BIGINT                | INTEGER               
 Name        | name         | VARCHAR(255) | VARCHAR(255)          | TEXT                  
 CreatedUnix | created_unix | BIGINT       | BIGINT                | INTEGER               

Primary keys: id
Indexes: 
	"IDX_attachment_issue_id" (issue_id)
	"IDX_attachment_release_id" (release_id)
	"UQE_attachment_uuid" UNIQUE (uuid)
```

# Table "collaboration"

```
 Field  | Column  |         PostgreSQL         |         MySQL          |          SQLite3           
--------+---------+----------------------------+------------------------+----------------------------
 ID     | id      | BIGSERIAL                  | BIGINT AUTO_INCREMENT  | INTEGER AUTOINCREMENT      
 UserID | user_id | BIGINT NOT NULL            | BIGINT NOT NULL        | INTEGER NOT NULL           
 RepoID | repo_id | BIGINT NOT NULL            | BIGINT NOT NULL        | INTEGER NOT NULL           
 Mode   | mode    | INTEGER NOT NULL DEFAULT 2 | INT NOT NULL DEFAULT 2 | INTEGER NOT NULL DEFAULT 2 

Primary keys: id
Indexes: 
	"IDX_collaboration_repo_id" (repo_id)
	"IDX_collaboration_user_id" (user_id)
	"UQE_collaboration_s" UNIQUE (user_id, repo_id)
```

# Table "comment"

```
    Field    |    Column    | PostgreSQL  |         MySQL         |        SQLite3        
-------------+--------------+-------------+-----------------------+-----------------------
 ID          | id           | BIGSERIAL   | BIGINT AUTO_INCREMENT | INTEGER AUTOINCREMENT 
 Type        | type         | INTEGER     | INT                   | INTEGER               
 PosterID    | poster_id    | BIGINT      | BIGINT                | INTEGER               
 IssueID     | issue_id     | BIGINT      | BIGINT                | INTEGER               
 CommitID    | commit_id    | BIGINT      | BIGINT                | INTEGER               
 Line        | line         | BIGINT      | BIGINT                | INTEGER               
 Content     | content      | TEXT        | TEXT                  | TEXT                  
 CreatedUnix | created_unix | BIGINT      | BIGINT                | INTEGER               
 UpdatedUnix | updated_unix | BIGINT      | BIGINT                | INTEGER               
 CommitSHA   | commit_sha   | VARCHAR(40) | VARCHAR(40)           | TEXT                  

Primary keys: id
Indexes: 
	"IDX_comment_issue_id" (issue_id)
```

# Table "deploy_key"

```
    Field    |    Column    |  PostgreSQL  |         MySQL         |        SQLite3        
-------------+--------------+--------------+-----------------------+-----------------------
 ID          | id           | BIGSERIAL    | BIGINT AUTO_INCREMENT | INTEGER AUTOINCREMENT 
 KeyID       | key_id       | BIGINT       | BIGINT                | INTEGER               
 RepoID      | repo_id      | BIGINT       | BIGINT                | INTEGER               
 Name        | name         | VARCHAR(255) | VARCHAR(255)          | TEXT                  
 Fingerprint | fingerprint  | VARCHAR(255) | VARCHAR(255)          | TEXT                  
 CreatedUnix | created_unix | BIGINT       | BIGINT                | INTEGER               
 UpdatedUnix | updated_unix | BIGINT       | BIGINT                | INTEGER               

Primary keys: id
Indexes: 
	"IDX_deploy_key_key_id" (key_id)
	"IDX_deploy_key_repo_id" (repo_id)
	"UQE_deploy_key_s" UNIQUE (key_id, repo_id)
```

# Table "email_address"

```
    Field    |    Column    |           PostgreSQL           |             MySQL              |            SQLite3             
-------------+--------------+--------------------------------+--------------------------------+--------------------------------
 ID          | id           | BIGSERIAL                      | BIGINT AUTO_INCREMENT          | INTEGER AUTOINCREMENT          
 UserID      | uid          | BIGINT NOT NULL                | BIGINT NOT NULL                | INTEGER NOT NULL               
 Email       | email        | VARCHAR(254) NOT NULL          | VARCHAR(254) NOT NULL          | TEXT NOT NULL                  
 IsActivated | is_activated | BOOLEAN NOT NULL DEFAULT FALSE | BOOLEAN NOT NULL DEFAULT FALSE | NUMERIC NOT NULL DEFAULT FALSE 

Primary keys: id
Indexes: 
	"email_address_user_email_unique" UNIQUE (uid, email)
	"idx_email_address_user_id" (uid)
```

# Table "follow"

```
  Field   |  Column   |   PostgreSQL    |         MySQL         |        SQLite3        
----------+-----------+-----------------+-----------------------+-----------------------
 ID       | id        | BIGSERIAL       | BIGINT AUTO_INCREMENT | INTEGER AUTOINCREMENT 
 UserID   | user_id   | BIGINT NOT NULL | BIGINT NOT NULL       | INTEGER NOT NULL      
 FollowID | follow_id | BIGINT NOT NULL | BIGINT NOT NULL       | INTEGER NOT NULL      

Primary keys: id
Indexes: 
	"follow_user_follow_unique" UNIQUE (user_id, follow_id)
```

# Table "hook_task"

```
      Field      |      Column      |  PostgreSQL  |         MySQL         |        SQLite3        
-----------------+------------------+--------------+-----------------------+-----------------------
 ID              | id               | BIGSERIAL    | BIGINT AUTO_INCREMENT | INTEGER AUTOINCREMENT 
 RepoID          | repo_id          | BIGINT       | BIGINT                | INTEGER               
 HookID          | hook_id          | BIGINT       | BIGINT                | INTEGER               
 UUID            | uuid             | VARCHAR(255) | VARCHAR(255)          | TEXT                  
 Type            | type             | INTEGER      | INT                   | INTEGER               
 URL             | url              | TEXT         | TEXT                  | TEXT                  
 Signature       | signature        | TEXT         | TEXT                  | TEXT                  
 PayloadContent  | payload_content  | TEXT         | TEXT                  | TEXT                  
 ContentType     | content_type     | INTEGER      | INT                   | INTEGER               
 EventType       | event_type       | VARCHAR(255) | VARCHAR(255)          | TEXT                  
 IsSSL           | is_ssl           | BOOLEAN      | BOOLEAN               | NUMERIC               
 IsDelivered     | is_delivered     | BOOLEAN      | BOOLEAN               | NUMERIC               
 Delivered       | delivered        | BIGINT       | BIGINT                | INTEGER               
 IsSucceed       | is_succeed       | BOOLEAN      | BOOLEAN               | NUMERIC               
 RequestContent  | request_content  | TEXT         | TEXT                  | TEXT                  
 ResponseContent | response_content | TEXT         | TEXT                  | TEXT                  

Primary keys: id
Indexes: 
	"IDX_hook_task_repo_id" (repo_id)
```

# Table "issue"

```
    Field     |    Column     |  PostgreSQL  |         MySQL         |        SQLite3        
--------------+---------------+--------------+-----------------------+-----------------------
 ID           | id            | BIGSERIAL    | BIGINT AUTO_INCREMENT | INTEGER AUTOINCREMENT 
 RepoID       | repo_id       | BIGINT       | BIGINT                | INTEGER               
 Index        | index         | BIGINT       | BIGINT                | INTEGER               
 PosterID     | poster_id     | BIGINT       | BIGINT                | INTEGER               
 Title        | name          | VARCHAR(255) | VARCHAR(255)          | TEXT                  
 Content      | content       | TEXT         | TEXT                  | TEXT                  
 MilestoneID  | milestone_id  | BIGINT       | BIGINT                | INTEGER               
 Priority     | priority      | INTEGER      | INT                   | INTEGER               
 AssigneeID   | assignee_id   | BIGINT       | BIGINT                | INTEGER               
 IsClosed     | is_closed     | BOOLEAN      | BOOLEAN               | NUMERIC               
 IsPull       | is_pull       | BOOLEAN      | BOOLEAN               | NUMERIC               
 NumComments  | num_comments  | INTEGER      | INT                   | INTEGER               
 DeadlineUnix | deadline_unix | BIGINT       | BIGINT                | INTEGER               
 CreatedUnix  | created_unix  | BIGINT       | BIGINT                | INTEGER               
 UpdatedUnix  | updated_unix  | BIGINT       | BIGINT                | INTEGER               

Primary keys: id
Indexes: 
	"IDX_issue_repo_id" (repo_id)
	"UQE_issue_repo_index" UNIQUE (repo_id, index)
```

# Table "issue_label"

```
  Field  |  Column  | PostgreSQL |         MySQL         |        SQLite3        
---------+----------+------------+-----------------------+-----------------------
 ID      | id       | BIGSERIAL  | BIGINT AUTO_INCREMENT | INTEGER AUTOINCREMENT 
 IssueID | issue_id | BIGINT     | BIGINT                | INTEGER               
 LabelID | label_id | BIGINT     | BIGINT                | INTEGER               

Primary keys: id
Indexes: 
	"UQE_issue_label_s" UNIQUE (issue_id, label_id)
```

# Table "issue_user"

```
    Field    |    Column    | PostgreSQL |         MySQL         |        SQLite3        
-------------+--------------+------------+-----------------------+-----------------------
 ID          | id           | BIGSERIAL  | BIGINT AUTO_INCREMENT | INTEGER AUTOINCREMENT 
 UserID      | uid          | BIGINT     | BIGINT                | INTEGER               
 IssueID     | issue_id     | BIGINT     | BIGINT                | INTEGER               
 RepoID      | repo_id      | BIGINT     | BIGINT                | INTEGER               
 MilestoneID | milestone_id | BIGINT     | BIGINT                | INTEGER               
 IsRead      | is_read      | BOOLEAN    | BOOLEAN               | NUMERIC               
 IsAssigned  | is_assigned  | BOOLEAN    | BOOLEAN               | NUMERIC               
 IsMentioned | is_mentioned | BOOLEAN    | BOOLEAN               | NUMERIC               
 IsPoster    | is_poster    | BOOLEAN    | BOOLEAN               | NUMERIC               
 IsClosed    | is_closed    | BOOLEAN    | BOOLEAN               | NUMERIC               

Primary keys: id
Indexes: 
	"IDX_issue_user_repo_id" (repo_id)
	"IDX_issue_user_uid" (uid)
```

# Table "label"

```
      Field      |      Column       |  PostgreSQL  |         MySQL         |        SQLite3        
-----------------+-------------------+--------------+-----------------------+-----------------------
 ID              | id                | BIGSERIAL    | BIGINT AUTO_INCREMENT | INTEGER AUTOINCREMENT 
 RepoID          | repo_id           | BIGINT       | BIGINT                | INTEGER               
 Name            | name              | VARCHAR(255) | VARCHAR(255)          | TEXT                  
 Color           | color             | VARCHAR(7)   | VARCHAR(7)            | TEXT                  
 NumIssues       | num_issues        | INTEGER      | INT                   | INTEGER               
 NumClosedIssues | num_closed_issues | INTEGER      | INT                   | INTEGER               

Primary keys: id
Indexes: 
	"IDX_label_repo_id" (repo_id)
```

# Table "lfs_object"

```
   Field   |   Column   |      PostgreSQL      |        MySQL         |      SQLite3      
-----------+------------+----------------------+----------------------+-------------------
 RepoID    | repo_id    | BIGINT               | BIGINT               | INTEGER           
 OID       | oid        | TEXT                 | VARCHAR(191)         | TEXT              
 Size      | size       | BIGINT NOT NULL      | BIGINT NOT NULL      | INTEGER NOT NULL  
 Storage   | storage    | TEXT NOT NULL        | LONGTEXT NOT NULL    | TEXT NOT NULL     
 CreatedAt | created_at | TIMESTAMPTZ NOT NULL | DATETIME(3) NOT NULL | DATETIME NOT NULL 

Primary keys: repo_id, oid
```

# Table "login_source"

```
    Field    |    Column    |    PostgreSQL    |         MySQL         |        SQLite3        
-------------+--------------+------------------+-----------------------+-----------------------
 ID          | id           | BIGSERIAL        | BIGINT AUTO_INCREMENT | INTEGER AUTOINCREMENT 
 Type        | type         | BIGINT           | BIGINT                | INTEGER               
 Name        | name         | TEXT UNIQUE      | VARCHAR(191) UNIQUE   | TEXT UNIQUE           
 IsActived   | is_actived   | BOOLEAN NOT NULL | BOOLEAN NOT NULL      | NUMERIC NOT NULL      
 IsDefault   | is_default   | BOOLEAN          | BOOLEAN               | NUMERIC               
 Config      | cfg          | TEXT             | TEXT                  | TEXT                  
 CreatedUnix | created_unix | BIGINT           | BIGINT                | INTEGER               
 UpdatedUnix | updated_unix | BIGINT           | BIGINT                | INTEGER               

Primary keys: id
```

# Table "milestone"

```
      Field      |      Column       |  PostgreSQL  |         MySQL         |        SQLite3        
-----------------+-------------------+--------------+-----------------------+-----------------------
 ID              | id                | BIGSERIAL    | BIGINT AUTO_INCREMENT | INTEGER AUTOINCREMENT 
 RepoID          | repo_id           | BIGINT       | BIGINT                | INTEGER               
 Name            | name              | VARCHAR(255) | VARCHAR(255)          | TEXT                  
 Content         | content           | TEXT         | TEXT                  | TEXT                  
 IsClosed        | is_closed         | BOOLEAN      | BOOLEAN               | NUMERIC               
 NumIssues       | num_issues        | INTEGER      | INT                   | INTEGER               
 NumClosedIssues | num_closed_issues | INTEGER      | INT                   | INTEGER               
 Completeness    | completeness      | INTEGER      | INT                   | INTEGER               
 DeadlineUnix    | deadline_unix     | BIGINT       | BIGINT                | INTEGER               
 ClosedDateUnix  | closed_date_unix  | BIGINT       | BIGINT                | INTEGER               

Primary keys: id
Indexes: 
	"IDX_milestone_repo_id" (repo_id)
```

# Table "mirror"

```
    Field     |      Column      |          PostgreSQL           |             MySQL             |            SQLite3            
--------------+------------------+-------------------------------+-------------------------------+-------------------------------
 ID           | id               | BIGSERIAL                     | BIGINT AUTO_INCREMENT         | INTEGER AUTOINCREMENT         
 RepoID       | repo_id          | BIGINT                        | BIGINT                        | INTEGER                       
 Interval     | interval         | INTEGER                       | INT                           | INTEGER                       
 EnablePrune  | enable_prune     | BOOLEAN NOT NULL DEFAULT TRUE | BOOLEAN NOT NULL DEFAULT TRUE | NUMERIC NOT NULL DEFAULT TRUE 
 LastSyncUnix | updated_unix     | BIGINT                        | BIGINT                        | INTEGER                       
 NextSyncUnix | next_update_unix | BIGINT                        | BIGINT                        | INTEGER                       

Primary keys: id
```

# Table "notice"

```
    Field    |    Column    | PostgreSQL |         MySQL         |        SQLite3        
-------------+--------------+------------+-----------------------+-----------------------
 ID          | id           | BIGSERIAL  | BIGINT AUTO_INCREMENT | INTEGER AUTOINCREMENT 
 Type        | type         | BIGINT     | BIGINT                | INTEGER               
 Description | description  | TEXT       | TEXT                  | TEXT                  
 CreatedUnix | created_unix | BIGINT     | BIGINT                | INTEGER               

Primary keys: id
```

# Table "org_user"

```
  Field   |  Column   | PostgreSQL |         MySQL         |        SQLite3        
----------+-----------+------------+-----------------------+-----------------------
 ID       | id        | BIGSERIAL  | BIGINT AUTO_INCREMENT | INTEGER AUTOINCREMENT 
 UID      | uid       | BIGINT     | BIGINT                | INTEGER               
 OrgID    | org_id    | BIGINT     | BIGINT                | INTEGER               
 IsPublic | is_public | BOOLEAN    | BOOLEAN               | NUMERIC               
 IsOwner  | is_owner  | BOOLEAN    | BOOLEAN               | NUMERIC               
 NumTeams | num_teams | INTEGER    | INT                   | INTEGER               

Primary keys: id
Indexes: 
	"IDX_org_user_org_id" (org_id)
	"IDX_org_user_uid" (uid)
	"UQE_org_user_s" UNIQUE (uid, org_id)
```

# Table "protect_branch"

```
       Field        |        Column        |  PostgreSQL  |         MySQL         |        SQLite3        
--------------------+----------------------+--------------+-----------------------+-----------------------
 ID                 | id                   | BIGSERIAL    | BIGINT AUTO_INCREMENT | INTEGER AUTOINCREMENT 
 RepoID             | repo_id              | BIGINT       | BIGINT                | INTEGER               
 Name               | name                 | VARCHAR(255) | VARCHAR(255)          | TEXT                  
 Protected          | protected            | BOOLEAN      | BOOLEAN               | NUMERIC               
 RequirePullRequest | require_pull_request | BOOLEAN      | BOOLEAN               | NUMERIC               
 EnableWhitelist    | enable_whitelist     | BOOLEAN      | BOOLEAN               | NUMERIC               
 WhitelistUserIDs   | whitelist_user_i_ds  | TEXT         | TEXT                  | TEXT                  
 WhitelistTeamIDs   | whitelist_team_i_ds  | TEXT         | TEXT                  | TEXT                  

Primary keys: id
Indexes: 
	"UQE_protect_branch_protect_branch" UNIQUE (repo_id, name)
```

# Table "protect_branch_whitelist"

```
      Field      |      Column       |  PostgreSQL  |         MySQL         |        SQLite3        
-----------------+-------------------+--------------+-----------------------+-----------------------
 ID              | id                | BIGSERIAL    | BIGINT AUTO_INCREMENT | INTEGER AUTOINCREMENT 
 ProtectBranchID | protect_branch_id | BIGINT       | BIGINT                | INTEGER               
 RepoID          | repo_id           | BIGINT       | BIGINT                | INTEGER               
 Name            | name              | VARCHAR(255) | VARCHAR(255)          | TEXT                  
 UserID          | user_id           | BIGINT       | BIGINT                | INTEGER               

Primary keys: id
Indexes: 
	"UQE_protect_branch_whitelist_protect_branch_whitelist" UNIQUE (repo_id, name, user_id)
```

# Table "public_key"

```
    Field    |    Column    |         PostgreSQL         |         MySQL          |          SQLite3           
-------------+--------------+----------------------------+------------------------+----------------------------
 ID          | id           | BIGSERIAL                  | BIGINT AUTO_INCREMENT  | INTEGER AUTOINCREMENT      
 OwnerID     | owner_id     | BIGINT NOT NULL            | BIGINT NOT NULL        | INTEGER NOT NULL           
 Name        | name         | VARCHAR(255) NOT NULL      | VARCHAR(255) NOT NULL  | TEXT NOT NULL              
 Fingerprint | fingerprint  | VARCHAR(255) NOT NULL      | VARCHAR(255) NOT NULL  | TEXT NOT NULL              
 Content     | content      | TEXT NOT NULL              | TEXT NOT NULL          | TEXT NOT NULL              
 Mode        | mode         | INTEGER NOT NULL DEFAULT 2 | INT NOT NULL DEFAULT 2 | INTEGER NOT NULL DEFAULT 2 
 Type        | type         | INTEGER NOT NULL DEFAULT 1 | INT NOT NULL DEFAULT 1 | INTEGER NOT NULL DEFAULT 1 
 CreatedUnix | created_unix | BIGINT                     | BIGINT                 | INTEGER                    
 UpdatedUnix | updated_unix | BIGINT                     | BIGINT                 | INTEGER                    

Primary keys: id
Indexes: 
	"IDX_public_key_owner_id" (owner_id)
```

# Table "pull_request"

```
     Field      |      Column      |  PostgreSQL  |         MySQL         |        SQLite3        
----------------+------------------+--------------+-----------------------+-----------------------
 ID             | id               | BIGSERIAL    | BIGINT AUTO_INCREMENT | INTEGER AUTOINCREMENT 
 Type           | type             | INTEGER      | INT                   | INTEGER               
 Status         | status           | INTEGER      | INT                   | INTEGER               
 IssueID        | issue_id         | BIGINT       | BIGINT                | INTEGER               
 Index          | index            | BIGINT       | BIGINT                | INTEGER               
 HeadRepoID     | head_repo_id     | BIGINT       | BIGINT                | INTEGER               
 BaseRepoID     | base_repo_id     | BIGINT       | BIGINT                | INTEGER               
 HeadUserName   | head_user_name   | VARCHAR(255) | VARCHAR(255)          | TEXT                  
 HeadBranch     | head_branch      | VARCHAR(255) | VARCHAR(255)          | TEXT                  
 BaseBranch     | base_branch      | VARCHAR(255) | VARCHAR(255)          | TEXT                  
 MergeBase      | merge_base       | VARCHAR(40)  | VARCHAR(40)           | TEXT                  
 HasMerged      | has_merged       | BOOLEAN      | BOOLEAN               | NUMERIC               
 MergedCommitID | merged_commit_id | VARCHAR(40)  | VARCHAR(40)           | TEXT                  
 MergerID       | merger_id        | BIGINT       | BIGINT                | INTEGER               
 MergedUnix     | merged_unix      | BIGINT       | BIGINT                | INTEGER               

Primary keys: id
Indexes: 
	"IDX_pull_request_issue_id" (issue_id)
```

# Table "release"

```
    Field     |     Column     |           PostgreSQL           |             MySQL              |            SQLite3             
--------------+----------------+--------------------------------+--------------------------------+--------------------------------
 ID           | id             | BIGSERIAL                      | BIGINT AUTO_INCREMENT          | INTEGER AUTOINCREMENT          
 RepoID       | repo_id        | BIGINT                         | BIGINT                         | INTEGER                        
 PublisherID  | publisher_id   | BIGINT                         | BIGINT                         | INTEGER                        
 TagName      | tag_name       | VARCHAR(255)                   | VARCHAR(255)                   | TEXT                           
 LowerTagName | lower_tag_name | VARCHAR(255)                   | VARCHAR(255)                   | TEXT                           
 Target       | target         | VARCHAR(255)                   | VARCHAR(255)                   | TEXT                           
 Title        | title          | VARCHAR(255)                   | VARCHAR(255)                   | TEXT                           
 Sha1         | sha1           | VARCHAR(40)                    | VARCHAR(40)                    | TEXT                           
 NumCommits   | num_commits    | BIGINT                         | BIGINT                         | INTEGER                        
 Note         | note           | TEXT                           | TEXT                           | TEXT                           
 IsDraft      | is_draft       | BOOLEAN NOT NULL DEFAULT FALSE | BOOLEAN NOT NULL DEFAULT FALSE | NUMERIC NOT NULL DEFAULT FALSE 
 IsPrerelease | is_prerelease  | BOOLEAN                        | BOOLEAN                        | NUMERIC                        
 CreatedUnix  | created_unix   | BIGINT                         | BIGINT                         | INTEGER                        

Primary keys: id
```

# Table "repository"

```
         Field         |         Column          |           PostgreSQL           |             MySQL              |            SQLite3             
-----------------------+-------------------------+--------------------------------+--------------------------------+--------------------------------
 ID                    | id                      | BIGSERIAL                      | BIGINT AUTO_INCREMENT          | INTEGER AUTOINCREMENT          
 OwnerID               | owner_id                | BIGINT                         | BIGINT                         | INTEGER                        
 LowerName             | lower_name              | VARCHAR(255) NOT NULL          | VARCHAR(255) NOT NULL          | TEXT NOT NULL                  
 Name                  | name                    | VARCHAR(255) NOT NULL          | VARCHAR(255) NOT NULL          | TEXT NOT NULL                  
 Description           | description             | VARCHAR(512)                   | VARCHAR(512)                   | TEXT                           
 Website               | website                 | VARCHAR(255)                   | VARCHAR(255)                   | TEXT                           
 DefaultBranch         | default_branch          | VARCHAR(255)                   | VARCHAR(255)                   | TEXT                           
 Size                  | size                    | BIGINT NOT NULL DEFAULT 0      | BIGINT NOT NULL DEFAULT 0      | INTEGER NOT NULL DEFAULT 0     
 UseCustomAvatar       | use_custom_avatar       | BOOLEAN                        | BOOLEAN                        | NUMERIC                        
 NumWatches            | num_watches             | INTEGER                        | INT                            | INTEGER                        
 NumStars              | num_stars               | INTEGER                        | INT                            | INTEGER                        
 NumForks              | num_forks               | INTEGER                        | INT                            | INTEGER                        
 NumIssues             | num_issues              | INTEGER                        | INT                            | INTEGER                        
 NumClosedIssues       | num_closed_issues       | INTEGER                        | INT                            | INTEGER                        
 NumPulls              | num_pulls               | INTEGER                        | INT                            | INTEGER                        
 NumClosedPulls        | num_closed_pulls        | INTEGER                        | INT                            | INTEGER                        
 NumMilestones         | num_milestones          | INTEGER NOT NULL DEFAULT 0     | INT NOT NULL DEFAULT 0         | INTEGER NOT NULL DEFAULT 0     
 NumClosedMilestones   | num_closed_milestones   | INTEGER NOT NULL DEFAULT 0     | INT NOT NULL DEFAULT 0         | INTEGER NOT NULL DEFAULT 0     
 IsPrivate             | is_private              | BOOLEAN                        | BOOLEAN                        | NUMERIC                        
 IsUnlisted            | is_unlisted             | BOOLEAN NOT NULL DEFAULT FALSE | BOOLEAN NOT NULL DEFAULT FALSE | NUMERIC NOT NULL DEFAULT FALSE 
 IsBare                | is_bare                 | BOOLEAN                        | BOOLEAN                        | NUMERIC                        
 IsMirror              | is_mirror               | BOOLEAN                        | BOOLEAN                        | NUMERIC                        
 EnableWiki            | enable_wiki             | BOOLEAN NOT NULL DEFAULT TRUE  | BOOLEAN NOT NULL DEFAULT TRUE  | NUMERIC NOT NULL DEFAULT TRUE  
 AllowPublicWiki       | allow_public_wiki       | BOOLEAN                        | BOOLEAN                        | NUMERIC                        
 EnableExternalWiki    | enable_external_wiki    | BOOLEAN                        | BOOLEAN                        | NUMERIC                        
 ExternalWikiURL       | external_wiki_url       | VARCHAR(255)                   | VARCHAR(255)                   | TEXT                           
 EnableIssues          | enable_issues           | BOOLEAN NOT NULL DEFAULT TRUE  | BOOLEAN NOT NULL DEFAULT TRUE  | NUMERIC NOT NULL DEFAULT TRUE  
 AllowPublicIssues     | allow_public_issues     | BOOLEAN                        | BOOLEAN                        | NUMERIC                        
 EnableExternalTracker | enable_external_tracker | BOOLEAN                        | BOOLEAN                        | NUMERIC                        
 ExternalTrackerURL    | external_tracker_url    | VARCHAR(255)                   | VARCHAR(255)                   | TEXT                           
 ExternalTrackerFormat | external_tracker_format | VARCHAR(255)                   | VARCHAR(255)                   | TEXT                           
 ExternalTrackerStyle  | external_tracker_style  | VARCHAR(255)                   | VARCHAR(255)                   | TEXT                           
 EnablePulls           | enable_pulls            | BOOLEAN NOT NULL DEFAULT TRUE  | BOOLEAN NOT NULL DEFAULT TRUE  | NUMERIC NOT NULL DEFAULT TRUE  
 PullsIgnoreWhitespace | pulls_ignore_whitespace | BOOLEAN NOT NULL DEFAULT FALSE | BOOLEAN NOT NULL DEFAULT FALSE | NUMERIC NOT NULL DEFAULT FALSE 
 PullsAllowRebase      | pulls_allow_rebase      | BOOLEAN NOT NULL DEFAULT FALSE | BOOLEAN NOT NULL DEFAULT FALSE | NUMERIC NOT NULL DEFAULT FALSE 
 IsFork                | is_fork                 | BOOLEAN NOT NULL DEFAULT FALSE | BOOLEAN NOT NULL DEFAULT FALSE | NUMERIC NOT NULL DEFAULT FALSE 
 ForkID                | fork_id                 | BIGINT                         | BIGINT                         | INTEGER                        
 CreatedUnix           | created_unix            | BIGINT                         | BIGINT                         | INTEGER                        
 UpdatedUnix           | updated_unix            | BIGINT                         | BIGINT                         | INTEGER                        

Primary keys: id
Indexes: 
	"IDX_repository_lower_name" (lower_name)
	"IDX_repository_name" (name)
	"UQE_repository_s" UNIQUE (owner_id, lower_name)
```

# Table "star"

```
 Field  | Column  | PostgreSQL |         MySQL         |        SQLite3        
--------+---------+------------+-----------------------+-----------------------
 ID     | id      | BIGSERIAL  | BIGINT AUTO_INCREMENT | INTEGER AUTOINCREMENT 
 UserID | uid     | BIGINT     | BIGINT                | INTEGER               
 RepoID | repo_id | BIGINT     | BIGINT                | INTEGER               

Primary keys: id
Indexes: 
	"UQE_star_s" UNIQUE (uid, repo_id)
```

# Table "team"

```
    Field    |   Column    |  PostgreSQL  |         MySQL         |        SQLite3        
-------------+-------------+--------------+-----------------------+-----------------------
 ID          | id          | BIGSERIAL    | BIGINT AUTO_INCREMENT | INTEGER AUTOINCREMENT 
 OrgID       | org_id      | BIGINT       | BIGINT                | INTEGER               
 LowerName   | lower_name  | VARCHAR(255) | VARCHAR(255)          | TEXT                  
 Name        | name        | VARCHAR(255) | VARCHAR(255)          | TEXT                  
 Description | description | VARCHAR(255) | VARCHAR(255)          | TEXT                  
 Authorize   | authorize   | INTEGER      | INT                   | INTEGER               
 NumRepos    | num_repos   | INTEGER      | INT                   | INTEGER               
 NumMembers  | num_members | INTEGER      | INT                   | INTEGER               

Primary keys: id
Indexes: 
	"IDX_team_org_id" (org_id)
```

# Table "team_repo"

```
 Field  | Column  | PostgreSQL |         MySQL         |        SQLite3        
--------+---------+------------+-----------------------+-----------------------
 ID     | id      | BIGSERIAL  | BIGINT AUTO_INCREMENT | INTEGER AUTOINCREMENT 
 OrgID  | org_id  | BIGINT     | BIGINT                | INTEGER               
 TeamID | team_id | BIGINT     | BIGINT                | INTEGER               
 RepoID | repo_id | BIGINT     | BIGINT                | INTEGER               

Primary keys: id
Indexes: 
	"IDX_team_repo_org_id" (org_id)
	"UQE_team_repo_s" UNIQUE (team_id, repo_id)
```

# Table "team_user"

```
 Field  | Column  | PostgreSQL |         MySQL         |        SQLite3        
--------+---------+------------+-----------------------+-----------------------
 ID     | id      | BIGSERIAL  | BIGINT AUTO_INCREMENT | INTEGER AUTOINCREMENT 
 OrgID  | org_id  | BIGINT     | BIGINT                | INTEGER               
 TeamID | team_id | BIGINT     | BIGINT                | INTEGER               
 UID    | uid     | BIGINT     | BIGINT                | INTEGER               

Primary keys: id
Indexes: 
	"IDX_team_user_org_id" (org_id)
	"UQE_team_user_s" UNIQUE (team_id, uid)
```

# Table "two_factor"

```
    Field    |    Column    |  PostgreSQL  |         MySQL         |        SQLite3        
-------------+--------------+--------------+-----------------------+-----------------------
 ID          | id           | BIGSERIAL    | BIGINT AUTO_INCREMENT | INTEGER AUTOINCREMENT 
 UserID      | user_id      | BIGINT       | BIGINT                | INTEGER               
 Secret      | secret       | VARCHAR(255) | VARCHAR(255)          | TEXT                  
 CreatedUnix | created_unix | BIGINT       | BIGINT                | INTEGER               

Primary keys: id
Indexes: 
	"UQE_two_factor_user_id" UNIQUE (user_id)
```

# Table "two_factor_recovery_code"

```
 Field  | Column  | PostgreSQL  |         MySQL         |        SQLite3        
--------+---------+-------------+-----------------------+-----------------------
 ID     | id      | BIGSERIAL   | BIGINT AUTO_INCREMENT | INTEGER AUTOINCREMENT 
 UserID | user_id | BIGINT      | BIGINT                | INTEGER               
 Code   | code    | VARCHAR(11) | VARCHAR(11)           | TEXT                  
 IsUsed | is_used | BOOLEAN     | BOOLEAN               | NUMERIC               

Primary keys: id
```

# Table "upload"

```
 Field | Column |  PostgreSQL  |         MySQL         |        SQLite3        
-------+--------+--------------+-----------------------+-----------------------
 ID    | id     | BIGSERIAL    | BIGINT AUTO_INCREMENT | INTEGER AUTOINCREMENT 
 UUID  | uuid   | VARCHAR(40)  | VARCHAR(40)           | TEXT                  
 Name  | name   | VARCHAR(255) | VARCHAR(255)          | TEXT                  

Primary keys: id
Indexes: 
	"UQE_upload_uuid" UNIQUE (uuid)
```

# Table "user"

```
       Field        |        Column        |         PostgreSQL          |           MySQL           |           SQLite3           
--------------------+----------------------+-----------------------------+---------------------------+-----------------------------
 ID                 | id                   | BIGSERIAL                   | BIGINT AUTO_INCREMENT     | INTEGER AUTOINCREMENT       
 LowerName          | lower_name           | VARCHAR(255) NOT NULL       | VARCHAR(255) NOT NULL     | TEXT NOT NULL               
 Name               | name                 | VARCHAR(255) NOT NULL       | VARCHAR(255) NOT NULL     | TEXT NOT NULL               
 FullName           | full_name            | VARCHAR(255)                | VARCHAR(255)              | TEXT                        
 Email              | email                | VARCHAR(255) NOT NULL       | VARCHAR(255) NOT NULL     | TEXT NOT NULL               
 Password           | passwd               | VARCHAR(255) NOT NULL       | VARCHAR(255) NOT NULL     | TEXT NOT NULL               
 LoginSource        | login_source         | BIGINT NOT NULL DEFAULT 0   | BIGINT NOT NULL DEFAULT 0 | INTEGER NOT NULL DEFAULT 0  
 LoginName          | login_name           | VARCHAR(255)                | VARCHAR(255)              | TEXT                        
 Type               | type                 | INTEGER                     | INT                       | INTEGER                     
 Location           | location             | VARCHAR(255)                | VARCHAR(255)              | TEXT                        
 Website            | website              | VARCHAR(255)                | VARCHAR(255)              | TEXT                        
 Rands              | rands                | VARCHAR(10)                 | VARCHAR(10)               | TEXT                        
 Salt               | salt                 | VARCHAR(10)                 | VARCHAR(10)               | TEXT                        
 CreatedUnix        | created_unix         | BIGINT                      | BIGINT                    | INTEGER                     
 UpdatedUnix        | updated_unix         | BIGINT                      | BIGINT                    | INTEGER                     
 LastRepoVisibility | last_repo_visibility | BOOLEAN                     | BOOLEAN                   | NUMERIC                     
 MaxRepoCreation    | max_repo_creation    | INTEGER NOT NULL DEFAULT -1 | INT NOT NULL DEFAULT -1   | INTEGER NOT NULL DEFAULT -1 
 IsActive           | is_active            | BOOLEAN                     | BOOLEAN                   | NUMERIC                     
 IsAdmin            | is_admin             | BOOLEAN                     | BOOLEAN                   | NUMERIC                     
 AllowGitHook       | allow_git_hook       | BOOLEAN                     | BOOLEAN                   | NUMERIC                     
 AllowImportLocal   | allow_import_local   | BOOLEAN                     | BOOLEAN                   | NUMERIC                     
 ProhibitLogin      | prohibit_login       | BOOLEAN                     | BOOLEAN                   | NUMERIC                     
 Avatar             | avatar               | VARCHAR(2048) NOT NULL      | VARCHAR(2048) NOT NULL    | TEXT NOT NULL               
 AvatarEmail        | avatar_email         | VARCHAR(255) NOT NULL       | VARCHAR(255) NOT NULL     | TEXT NOT NULL               
 UseCustomAvatar    | use_custom_avatar    | BOOLEAN                     | BOOLEAN                   | NUMERIC                     
 NumFollowers       | num_followers        | INTEGER                     | INT                       | INTEGER                     
 NumFollowing       | num_following        | INTEGER NOT NULL DEFAULT 0  | INT NOT NULL DEFAULT 0    | INTEGER NOT NULL DEFAULT 0  
 NumStars           | num_stars            | INTEGER                     | INT                       | INTEGER                     
 NumRepos           | num_repos            | INTEGER                     | INT                       | INTEGER                     
 Description        | description          | VARCHAR(255)                | VARCHAR(255)              | TEXT                        
 NumTeams           | num_teams            | INTEGER                     | INT                       | INTEGER                     
 NumMembers         | num_members          | INTEGER                     | INT                       | INTEGER                     

Primary keys: id
Indexes: 
	"UQE_user_lower_name" UNIQUE (lower_name)
	"UQE_user_name" UNIQUE (name)
```

# Table "watch"

```
 Field  | Column  | PostgreSQL |         MySQL         |        SQLite3        
--------+---------+------------+-----------------------+-----------------------
 ID     | id      | BIGSERIAL  | BIGINT AUTO_INCREMENT | INTEGER AUTOINCREMENT 
 UserID | user_id | BIGINT     | BIGINT                | INTEGER               
 RepoID | repo_id | BIGINT     | BIGINT                | INTEGER               

Primary keys: id
Indexes: 
	"UQE_watch_watch" UNIQUE (user_id, repo_id)
```

# Table "webhook"

```
    Field     |     Column     | PostgreSQL |         MySQL         |        SQLite3        
--------------+----------------+------------+-----------------------+-----------------------
 ID           | id             | BIGSERIAL  | BIGINT AUTO_INCREMENT | INTEGER AUTOINCREMENT 
 RepoID       | repo_id        | BIGINT     | BIGINT                | INTEGER               
 OrgID        | org_id         | BIGINT     | BIGINT                | INTEGER               
 URL          | url            | TEXT       | TEXT                  | TEXT                  
 ContentType  | content_type   | INTEGER    | INT                   | INTEGER               
 Secret       | secret         | TEXT       | TEXT                  | TEXT                  
 Events       | events         | TEXT       | TEXT                  | TEXT                  
 IsSSL        | is_ssl         | BOOLEAN    | BOOLEAN               | NUMERIC               
 IsActive     | is_active      | BOOLEAN    | BOOLEAN               | NUMERIC               
 HookTaskType | hook_task_type | INTEGER    | INT                   | INTEGER               
 Meta         | meta           | TEXT       | TEXT                  | TEXT                  
 LastStatus   | last_status    | INTEGER    | INT                   | INTEGER               
 CreatedUnix  | created_unix   | BIGINT     | BIGINT                | INTEGER               
 UpdatedUnix  | updated_unix   | BIGINT     | BIGINT                | INTEGER               

Primary keys: id
```

