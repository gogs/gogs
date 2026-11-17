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

# Table "passkey"

```
    Field    |    Column    |   PostgreSQL    |         MySQL         |        SQLite3        
-------------+--------------+-----------------+-----------------------+-----------------------
 ID          | id           | BIGSERIAL       | BIGINT AUTO_INCREMENT | INTEGER AUTOINCREMENT 
 UserID      | user_id      | BIGINT NOT NULL | BIGINT NOT NULL       | INTEGER NOT NULL      
 Name        | name         | TEXT NOT NULL   | LONGTEXT NOT NULL     | TEXT NOT NULL         
 Credential  | credential   | TEXT NOT NULL   | TEXT NOT NULL         | TEXT NOT NULL         
 CreatedUnix | created_unix | BIGINT          | BIGINT                | INTEGER               
 UpdatedUnix | updated_unix | BIGINT          | BIGINT                | INTEGER               

Primary keys: id
Indexes: 
	"idx_passkey_user_id" (user_id)
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

