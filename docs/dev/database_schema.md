# Table "access"

```
  FIELD  | COLUMN  |   POSTGRESQL    |         MYSQL         |     SQLITE3       
---------+---------+-----------------+-----------------------+-------------------
  ID     | id      | BIGSERIAL       | BIGINT AUTO_INCREMENT | INTEGER           
  UserID | user_id | BIGINT NOT NULL | BIGINT NOT NULL       | INTEGER NOT NULL  
  RepoID | repo_id | BIGINT NOT NULL | BIGINT NOT NULL       | INTEGER NOT NULL  
  Mode   | mode    | BIGINT NOT NULL | BIGINT NOT NULL       | INTEGER NOT NULL  

Primary keys: id
Indexes: 
	"access_user_repo_unique" UNIQUE (user_id, repo_id)
```

# Table "access_token"

```
     FIELD    |    COLUMN    |         POSTGRESQL          |            MYSQL            |           SQLITE3            
--------------+--------------+-----------------------------+-----------------------------+------------------------------
  ID          | id           | BIGSERIAL                   | BIGINT AUTO_INCREMENT       | INTEGER                      
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
     FIELD     |     COLUMN     |           POSTGRESQL           |             MYSQL              |            SQLITE3              
---------------+----------------+--------------------------------+--------------------------------+---------------------------------
  ID           | id             | BIGSERIAL                      | BIGINT AUTO_INCREMENT          | INTEGER                         
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

# Table "email_address"

```
     FIELD    |    COLUMN    |           POSTGRESQL           |             MYSQL              |            SQLITE3              
--------------+--------------+--------------------------------+--------------------------------+---------------------------------
  ID          | id           | BIGSERIAL                      | BIGINT AUTO_INCREMENT          | INTEGER                         
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
   FIELD   |  COLUMN   |   POSTGRESQL    |         MYSQL         |     SQLITE3       
-----------+-----------+-----------------+-----------------------+-------------------
  ID       | id        | BIGSERIAL       | BIGINT AUTO_INCREMENT | INTEGER           
  UserID   | user_id   | BIGINT NOT NULL | BIGINT NOT NULL       | INTEGER NOT NULL  
  FollowID | follow_id | BIGINT NOT NULL | BIGINT NOT NULL       | INTEGER NOT NULL  

Primary keys: id
Indexes: 
	"follow_user_follow_unique" UNIQUE (user_id, follow_id)
```

# Table "lfs_object"

```
    FIELD   |   COLUMN   |      POSTGRESQL      |        MYSQL         |      SQLITE3       
------------+------------+----------------------+----------------------+--------------------
  RepoID    | repo_id    | BIGINT               | BIGINT               | INTEGER            
  OID       | oid        | TEXT                 | VARCHAR(191)         | TEXT               
  Size      | size       | BIGINT NOT NULL      | BIGINT NOT NULL      | INTEGER NOT NULL   
  Storage   | storage    | TEXT NOT NULL        | LONGTEXT NOT NULL    | TEXT NOT NULL      
  CreatedAt | created_at | TIMESTAMPTZ NOT NULL | DATETIME(3) NOT NULL | DATETIME NOT NULL  

Primary keys: repo_id, oid
```

# Table "login_source"

```
     FIELD    |    COLUMN    |    POSTGRESQL    |         MYSQL         |     SQLITE3       
--------------+--------------+------------------+-----------------------+-------------------
  ID          | id           | BIGSERIAL        | BIGINT AUTO_INCREMENT | INTEGER           
  Type        | type         | BIGINT           | BIGINT                | INTEGER           
  Name        | name         | TEXT UNIQUE      | VARCHAR(191) UNIQUE   | TEXT UNIQUE       
  IsActived   | is_actived   | BOOLEAN NOT NULL | BOOLEAN NOT NULL      | NUMERIC NOT NULL  
  IsDefault   | is_default   | BOOLEAN          | BOOLEAN               | NUMERIC           
  Config      | cfg          | TEXT             | TEXT                  | TEXT              
  CreatedUnix | created_unix | BIGINT           | BIGINT                | INTEGER           
  UpdatedUnix | updated_unix | BIGINT           | BIGINT                | INTEGER           

Primary keys: id
```

