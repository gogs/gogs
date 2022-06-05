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

