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
-- Table structure for table `tbl_dbaas_system_config`
--

DROP TABLE IF EXISTS `tbl_dbaas_system_config`;
/*!40101 SET @saved_cs_client     = @@character_set_client */;
/*!40101 SET character_set_client = utf8 */;
CREATE TABLE `tbl_dbaas_system_config` (
  `ai` int(24) NOT NULL AUTO_INCREMENT COMMENT '自增字段,与业务无关',
  `dc_id` int(11) NOT NULL COMMENT '站点ID',
  `retry` tinyint(4) DEFAULT NULL COMMENT '资源分配失败重试次数',
  `consul_ip` varchar(128) NOT NULL COMMENT 'Consul server IP地址, 包含多个IP',
  `consul_port` int(10) unsigned NOT NULL COMMENT 'Consul Server HTTP Port',
  `consul_dc` varchar(45) NOT NULL COMMENT 'Consul DataCenter',
  `consul_token` varchar(45) DEFAULT NULL COMMENT 'Consul Token,可以为空',
  `consul_wait_time` int(10) DEFAULT NULL,
  `docker_port` int(10) unsigned NOT NULL COMMENT 'docker Port',
  `plugin_port` int(6) NOT NULL COMMENT 'docker volume plugin port',
  `horus_agent_port` int(11) NOT NULL,
  `registry_os_username` varchar(45) NOT NULL COMMENT 'registry 操作系统用户',
  `registry_os_password` varchar(45) NOT NULL COMMENT 'registry 操作系统用户密码',
  `registry_domain` varchar(45) NOT NULL COMMENT 'registry 域名',
  `registry_ip` varchar(45) NOT NULL COMMENT 'registry IP地址',
  `registry_port` int(6) NOT NULL COMMENT 'registry 端口',
  `registry_username` varchar(45) NOT NULL COMMENT 'registry 用户名',
  `registry_password` varchar(45) NOT NULL COMMENT 'registry 用户密码',
  `registry_email` varchar(128) NOT NULL COMMENT 'registry 邮箱',
  `registry_ca_crt` text NOT NULL COMMENT 'registry证书文件内容',
  `registry_token` varchar(4096) DEFAULT NULL,
  `source_dir` varchar(128) NOT NULL COMMENT '物理机初始化包绝对路径',
  `destination_dir` varchar(128) NOT NULL COMMENT '物理机初始化包远程目标目录',
  `init_script_name` varchar(45) NOT NULL COMMENT '物理机入库初始化脚本名',
  `clean_script_name` varchar(45) NOT NULL COMMENT '物理机出库清理脚本名',
  `ca_crt_name` varchar(45) NOT NULL COMMENT '证书文件名称',
  `nfs_ip` varchar(45) NOT NULL COMMENT 'nfs IP地址',
  `nfs_dir` varchar(128) NOT NULL COMMENT 'nfs 源目录',
  `nfs_mount_dir` varchar(128) NOT NULL COMMENT 'nfs 挂载目录',
  `nfs_mount_opts` varchar(128) DEFAULT NULL COMMENT 'nfs 挂载参数',
  `backup_dir` varchar(128) NOT NULL COMMENT '挂载到容器内的备份目录',
  `check_username` varchar(128) NOT NULL,
  `check_password` varchar(128) NOT NULL,
  `mon_username` varchar(128) NOT NULL COMMENT '数据库监控用户',
  `mon_password` varchar(128) NOT NULL COMMENT '数据库监控用户密码',
  `repl_username` varchar(128) NOT NULL COMMENT '数据库数据复制用户',
  `repl_password` varchar(128) NOT NULL COMMENT '数据库数据复制用户密码',
  `cup_dba_username` varchar(128) NOT NULL COMMENT '数据库超级用户',
  `cup_dba_password` varchar(128) NOT NULL COMMENT '数据库超级用户密码',
  `db_username` varchar(128) NOT NULL COMMENT '数据库管理用户,用于proxy用户映射权限db',
  `db_password` varchar(128) NOT NULL,
  `ap_username` varchar(128) NOT NULL COMMENT '数据库应用用户,用于proxy用户映射权限ap',
  `ap_password` varchar(128) NOT NULL,
  PRIMARY KEY (`ai`),
  UNIQUE KEY `dc_id_UNIQUE` (`dc_id`)
) ENGINE=InnoDB AUTO_INCREMENT=2 DEFAULT CHARSET=utf8;
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
