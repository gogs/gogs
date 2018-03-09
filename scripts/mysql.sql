SET GLOBAL innodb_file_per_table = ON,
           innodb_file_format = Barracuda,
           innodb_large_prefix = ON;
DROP DATABASE IF EXISTS gogs;
CREATE DATABASE IF NOT EXISTS gogs CHARACTER SET utf8mb4 COLLATE utf8mb4_general_ci;
