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
	"idx_access_repo_id" (repo_id)
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

# Table "background_jobs"

```
     Field      |      Column      |            PostgreSQL            |              MySQL               |             SQLite3              
----------------+------------------+----------------------------------+----------------------------------+----------------------------------
 ID             | id               | BIGSERIAL                        | BIGINT AUTO_INCREMENT            | INTEGER AUTOINCREMENT            
 Kind           | kind             | VARCHAR(32) NOT NULL             | VARCHAR(32) NOT NULL             | VARCHAR(32) NOT NULL             
 UniqueKey      | unique_key       | VARCHAR(255) NOT NULL DEFAULT '' | VARCHAR(255) NOT NULL DEFAULT '' | VARCHAR(255) NOT NULL DEFAULT "" 
 Payload        | payload          | BYTEA NOT NULL                   | LONGBLOB NOT NULL                | BLOB NOT NULL                    
 State          | state            | VARCHAR(16) NOT NULL             | VARCHAR(16) NOT NULL             | VARCHAR(16) NOT NULL             
 Priority       | priority         | SMALLINT NOT NULL DEFAULT 100    | SMALLINT NOT NULL DEFAULT 100    | SMALLINT NOT NULL DEFAULT 100    
 Attempts       | attempts         | BIGINT NOT NULL DEFAULT 0        | BIGINT NOT NULL DEFAULT 0        | INTEGER NOT NULL DEFAULT 0       
 MaxAttempts    | max_attempts     | BIGINT NOT NULL                  | BIGINT NOT NULL                  | INTEGER NOT NULL                 
 RunAt          | run_at           | TIMESTAMPTZ NOT NULL             | DATETIME(3) NOT NULL             | DATETIME NOT NULL                
 LeaseExpiresAt | lease_expires_at | TIMESTAMPTZ                      | DATETIME(3) NULL                 | DATETIME                         
 LastError      | last_error       | TEXT                             | TEXT                             | TEXT                             
 CreatedAt      | created_at       | TIMESTAMPTZ NOT NULL             | DATETIME(3) NOT NULL             | DATETIME NOT NULL                
 UpdatedAt      | updated_at       | TIMESTAMPTZ NOT NULL             | DATETIME(3) NOT NULL             | DATETIME NOT NULL                

Primary keys: id
Indexes: 
	"idx_background_jobs_kind_state_priority_run_at" (kind, state, priority, run_at)
	"idx_background_jobs_kind_unique_key" (kind, unique_key)
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

