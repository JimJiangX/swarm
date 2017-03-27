-- MySQL dump 10.13  Distrib 5.7.12, for Win32 (AMD64)
--
-- Host: 146.240.104.26    Database: dbaas
-- ------------------------------------------------------
-- Server version	5.6.19-log

/*!40101 SET @OLD_CHARACTER_SET_CLIENT=@@CHARACTER_SET_CLIENT */;
/*!40101 SET @OLD_CHARACTER_SET_RESULTS=@@CHARACTER_SET_RESULTS */;
/*!40101 SET @OLD_COLLATION_CONNECTION=@@COLLATION_CONNECTION */;
/*!40101 SET NAMES utf8 */;
/*!40103 SET @OLD_TIME_ZONE=@@TIME_ZONE */;
/*!40103 SET TIME_ZONE='+00:00' */;
/*!40014 SET @OLD_UNIQUE_CHECKS=@@UNIQUE_CHECKS, UNIQUE_CHECKS=0 */;
/*!40014 SET @OLD_FOREIGN_KEY_CHECKS=@@FOREIGN_KEY_CHECKS, FOREIGN_KEY_CHECKS=0 */;
/*!40101 SET @OLD_SQL_MODE=@@SQL_MODE, SQL_MODE='NO_AUTO_VALUE_ON_ZERO' */;
/*!40111 SET @OLD_SQL_NOTES=@@SQL_NOTES, SQL_NOTES=0 */;
SET @MYSQLDUMP_TEMP_LOG_BIN = @@SESSION.SQL_LOG_BIN;
SET @@SESSION.SQL_LOG_BIN= 0;

--
-- GTID state at the beginning of the backup 
--

SET @@GLOBAL.GTID_PURGED='7ed61159-7fc9-11e6-9423-008cfae833f0:1-153,
99210b42-7fc9-11e6-9423-008cfaecf318:1-12';

--
-- Table structure for table `tbl_dbaas_users`
--

DROP TABLE IF EXISTS `tbl_dbaas_users`;
/*!40101 SET @saved_cs_client     = @@character_set_client */;
/*!40101 SET character_set_client = utf8 */;
CREATE TABLE `tbl_dbaas_users` (
  `ai` int(24) NOT NULL AUTO_INCREMENT COMMENT '自增字段,与业务无关',
  `id` varchar(128) NOT NULL COMMENT '数据库用户ID',
  `service_id` varchar(128) NOT NULL COMMENT '所属服务ID',
  `type` varchar(45) NOT NULL COMMENT '用户类型\nupsql:数据库\nupproxy:代理',
  `username` varchar(128) NOT NULL COMMENT '用户名',
  `password` varchar(128) DEFAULT NULL COMMENT '密码,暂时使用明文密码',
  `role` varchar(45) NOT NULL COMMENT '用户权限角色,用于对应数据库用户名称\ncup_db\ndb\nap',
  `blacklist` varchar(1024) DEFAULT NULL COMMENT '黑名单',
  `whitelist` varchar(1024) DEFAULT NULL COMMENT '白名单',
  `created_at` datetime NOT NULL,
  `read_only` tinyint(3) DEFAULT '0' COMMENT '只读',
  `rw_split` tinyint(3) DEFAULT '0' COMMENT '读写分离',
  `shard` tinyint(3) DEFAULT '0' COMMENT '分库分表',
  PRIMARY KEY (`ai`),
  UNIQUE KEY `id_UNIQUE` (`id`)
) ENGINE=InnoDB AUTO_INCREMENT=1950 DEFAULT CHARSET=utf8;
/*!40101 SET character_set_client = @saved_cs_client */;
SET @@SESSION.SQL_LOG_BIN = @MYSQLDUMP_TEMP_LOG_BIN;
/*!40103 SET TIME_ZONE=@OLD_TIME_ZONE */;

/*!40101 SET SQL_MODE=@OLD_SQL_MODE */;
/*!40014 SET FOREIGN_KEY_CHECKS=@OLD_FOREIGN_KEY_CHECKS */;
/*!40014 SET UNIQUE_CHECKS=@OLD_UNIQUE_CHECKS */;
/*!40101 SET CHARACTER_SET_CLIENT=@OLD_CHARACTER_SET_CLIENT */;
/*!40101 SET CHARACTER_SET_RESULTS=@OLD_CHARACTER_SET_RESULTS */;
/*!40101 SET COLLATION_CONNECTION=@OLD_COLLATION_CONNECTION */;
/*!40111 SET SQL_NOTES=@OLD_SQL_NOTES */;

-- Dump completed on 2017-01-04 14:44:17
