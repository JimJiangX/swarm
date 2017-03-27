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
-- Table structure for table `tbl_dbaas_service`
--

DROP TABLE IF EXISTS `tbl_dbaas_service`;
/*!40101 SET @saved_cs_client     = @@character_set_client */;
/*!40101 SET character_set_client = utf8 */;
CREATE TABLE `tbl_dbaas_service` (
  `ai` int(24) NOT NULL AUTO_INCREMENT COMMENT '自增字段,与业务无关',
  `id` varchar(128) NOT NULL COMMENT '服务ID',
  `name` varchar(128) NOT NULL COMMENT '服务名称',
  `business_code` varchar(128) NOT NULL COMMENT '子系统代码',
  `description` text NOT NULL COMMENT '服务描述信息',
  `architecture` varchar(128) NOT NULL COMMENT '服务结构描述:\n<软件名称>:<数量>#<软件名称>:<数量>\n例:switch_manager:1#proxy:1#upsql:2',
  `auto_healing` tinyint(1) unsigned DEFAULT '0' COMMENT '服务自动愈合\n0:停用\n1:启用',
  `auto_scaling` tinyint(1) unsigned DEFAULT '0' COMMENT '自动扩缩\n0:停用\n1:启用',
  `backup_max_size` bigint(128) unsigned DEFAULT NULL COMMENT '备份文件总大小,单位:byte',
  `backup_files_retention` bigint(128) DEFAULT NULL COMMENT '文件保存时间,单位:Hour',
  `status` tinyint(4) unsigned DEFAULT NULL COMMENT '管理状态\n0	已分配\n1	创建中\n2	启动中\n3	停止中\n4	删除中\n5	备份中\n6	还原中\n99	无任务',
  `created_at` datetime NOT NULL COMMENT '创建日期',
  `finished_at` datetime DEFAULT NULL COMMENT '创建完成日期',
  PRIMARY KEY (`ai`),
  UNIQUE KEY `id_UNIQUE` (`id`),
  UNIQUE KEY `name_UNIQUE` (`name`)
) ENGINE=InnoDB AUTO_INCREMENT=279 DEFAULT CHARSET=utf8;
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
