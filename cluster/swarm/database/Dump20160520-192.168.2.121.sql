CREATE DATABASE  IF NOT EXISTS `DBaaS` /*!40100 DEFAULT CHARACTER SET utf8 */;
USE `DBaaS`;
-- MySQL dump 10.13  Distrib 5.6.22, for osx10.8 (x86_64)
--
-- Host: 192.168.2.121    Database: DBaaS
-- ------------------------------------------------------
-- Server version	5.5.5-10.0.21-MariaDB

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

--
-- Table structure for table `deltb_container`
--

DROP TABLE IF EXISTS `deltb_container`;
/*!40101 SET @saved_cs_client     = @@character_set_client */;
/*!40101 SET character_set_client = utf8 */;
CREATE TABLE `deltb_container` (
  `id` varchar(128) NOT NULL COMMENT 'docker 生成的ID',
  `node_id` varchar(128) NOT NULL COMMENT '所属 POT ID',
  `ip_addr` varchar(45) NOT NULL COMMENT 'Container IP Address',
  `networking_id` varchar(45) NOT NULL,
  `ports` varchar(128) DEFAULT NULL COMMENT 'ports:”public”:xxx;”private”:xxx;',
  `image` varchar(128) NOT NULL COMMENT 'image ID',
  `ncpu` int(11) NOT NULL COMMENT '指定CPU核数',
  `cpu_set` varchar(45) DEFAULT NULL COMMENT '指定容器亲缘cpu',
  `cpu_shares` int(11) DEFAULT NULL,
  `mem` int(11) unsigned DEFAULT NULL COMMENT '指定可使用Memory大小，单位Byte\n0为无限制',
  `mem_swap` int(11) unsigned DEFAULT NULL COMMENT '单位：MB',
  `network_mode` varchar(45) NOT NULL COMMENT '网络模式',
  `storage_type` varchar(45) NOT NULL COMMENT 'Container 数据存储类型',
  `volume_driver` varchar(45) NOT NULL,
  `volumes_from` varchar(1024) NOT NULL COMMENT '[{id:<volume_id>,type:data},{id:<volume_id>,type:logs}]',
  `filesystem` varchar(45) DEFAULT NULL COMMENT '文件系统类型',
  `env` varchar(256) DEFAULT NULL,
  `cmd` varchar(1024) DEFAULT NULL,
  `created_at` datetime NOT NULL,
  PRIMARY KEY (`id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8;
/*!40101 SET character_set_client = @saved_cs_client */;

--
-- Dumping data for table `deltb_container`
--

LOCK TABLES `deltb_container` WRITE;
/*!40000 ALTER TABLE `deltb_container` DISABLE KEYS */;
/*!40000 ALTER TABLE `deltb_container` ENABLE KEYS */;
UNLOCK TABLES;

--
-- Table structure for table `tb_backup_files`
--

DROP TABLE IF EXISTS `tb_backup_files`;
/*!40101 SET @saved_cs_client     = @@character_set_client */;
/*!40101 SET character_set_client = utf8 */;
CREATE TABLE `tb_backup_files` (
  `id` varchar(128) NOT NULL,
  `strategy_id` varchar(128) NOT NULL COMMENT '关联的备份策略id',
  `task_id` varchar(128) NOT NULL COMMENT '关联tb_task.id',
  `unit_id` varchar(128) NOT NULL COMMENT '所属容器的id',
  `type` varchar(45) DEFAULT NULL COMMENT '全量／增量\n\nfull/incremental',
  `path` varchar(1024) DEFAULT NULL COMMENT '备份文件路径（包含文件名）',
  `size` int(10) unsigned DEFAULT NULL COMMENT '备份文件大小，单位：MB',
  `retention` datetime DEFAULT NULL,
  `created_at` datetime NOT NULL COMMENT '创建时间',
  PRIMARY KEY (`id`),
  UNIQUE KEY `id_UNIQUE` (`id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8;
/*!40101 SET character_set_client = @saved_cs_client */;

--
-- Dumping data for table `tb_backup_files`
--

LOCK TABLES `tb_backup_files` WRITE;
/*!40000 ALTER TABLE `tb_backup_files` DISABLE KEYS */;
/*!40000 ALTER TABLE `tb_backup_files` ENABLE KEYS */;
UNLOCK TABLES;

--
-- Table structure for table `tb_backup_strategy`
--

DROP TABLE IF EXISTS `tb_backup_strategy`;
/*!40101 SET @saved_cs_client     = @@character_set_client */;
/*!40101 SET character_set_client = utf8 */;
CREATE TABLE `tb_backup_strategy` (
  `id` varchar(128) NOT NULL COMMENT '备份策略ID',
  `service_id` varchar(128) NOT NULL COMMENT '所属服务ID',
  `spec` varchar(45) NOT NULL COMMENT '触发规则描述，cron语法',
  `next` datetime DEFAULT NULL COMMENT '下一次执行时间',
  `valid` datetime NOT NULL COMMENT '有效期限',
  `timeout` bigint(128) unsigned DEFAULT NULL COMMENT '执行备份的超时时长，time.Unix()值。',
  `backup_dir` varchar(128) NOT NULL COMMENT '备份目录',
  `type` varchar(64) DEFAULT NULL COMMENT '备份类型\n全量／增量\nfull／incremental\n',
  `enabled` tinyint(1) unsigned DEFAULT '1' COMMENT '0:停用，1：启用',
  `created_at` datetime DEFAULT NULL COMMENT '创建时间',
  PRIMARY KEY (`id`),
  UNIQUE KEY `id_UNIQUE` (`id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8;
/*!40101 SET character_set_client = @saved_cs_client */;

--
-- Dumping data for table `tb_backup_strategy`
--

LOCK TABLES `tb_backup_strategy` WRITE;
/*!40000 ALTER TABLE `tb_backup_strategy` DISABLE KEYS */;
/*!40000 ALTER TABLE `tb_backup_strategy` ENABLE KEYS */;
UNLOCK TABLES;

--
-- Table structure for table `tb_cluster`
--

DROP TABLE IF EXISTS `tb_cluster`;
/*!40101 SET @saved_cs_client     = @@character_set_client */;
/*!40101 SET character_set_client = utf8 */;
CREATE TABLE `tb_cluster` (
  `id` varchar(128) NOT NULL COMMENT '主键',
  `name` varchar(128) NOT NULL COMMENT 'cluster name',
  `type` varchar(64) NOT NULL COMMENT 'upsql / upproxy',
  `storage_type` varchar(64) NOT NULL COMMENT 'local, HUAWEI, HITACHI',
  `storage_id` varchar(128) DEFAULT NULL COMMENT '存储系统ID',
  `max_node` int(11) unsigned NOT NULL DEFAULT '500' COMMENT '最多物理机数量',
  `usage_limit` float NOT NULL DEFAULT '0.8' COMMENT '物理机资源使用上限',
  `datacenter` varchar(64) NOT NULL COMMENT '数据中心名称',
  `enabled` tinyint(1) unsigned NOT NULL DEFAULT '1' COMMENT '集群状态，0：停用，1：启用。',
  PRIMARY KEY (`id`),
  UNIQUE KEY `id_UNIQUE` (`id`),
  UNIQUE KEY `name_UNIQUE` (`name`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8;
/*!40101 SET character_set_client = @saved_cs_client */;

--
-- Dumping data for table `tb_cluster`
--

LOCK TABLES `tb_cluster` WRITE;
/*!40000 ALTER TABLE `tb_cluster` DISABLE KEYS */;
/*!40000 ALTER TABLE `tb_cluster` ENABLE KEYS */;
UNLOCK TABLES;

--
-- Table structure for table `tb_image`
--

DROP TABLE IF EXISTS `tb_image`;
/*!40101 SET @saved_cs_client     = @@character_set_client */;
/*!40101 SET character_set_client = utf8 */;
CREATE TABLE `tb_image` (
  `id` varchar(128) NOT NULL COMMENT '主键',
  `name` varchar(128) NOT NULL COMMENT '软件包名称',
  `size` int(64) DEFAULT NULL,
  `docker_image_id` varchar(128) DEFAULT NULL COMMENT 'docker image id',
  `version` varchar(128) NOT NULL COMMENT '软件版本号',
  `template_config_id` varchar(128) DEFAULT NULL COMMENT '配置文件模版ID',
  `label` varchar(4096) DEFAULT NULL COMMENT '备注',
  `enabled` tinyint(1) unsigned DEFAULT '1' COMMENT '可用状态，0：停用，1：启用。',
  `upload_at` datetime NOT NULL COMMENT '上传日期',
  PRIMARY KEY (`id`),
  UNIQUE KEY `id_UNIQUE` (`id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8;
/*!40101 SET character_set_client = @saved_cs_client */;

--
-- Dumping data for table `tb_image`
--

LOCK TABLES `tb_image` WRITE;
/*!40000 ALTER TABLE `tb_image` DISABLE KEYS */;
/*!40000 ALTER TABLE `tb_image` ENABLE KEYS */;
UNLOCK TABLES;

--
-- Table structure for table `tb_ip`
--

DROP TABLE IF EXISTS `tb_ip`;
/*!40101 SET @saved_cs_client     = @@character_set_client */;
/*!40101 SET character_set_client = utf8 */;
CREATE TABLE `tb_ip` (
  `ip_addr` int(11) unsigned NOT NULL COMMENT 'IP地址,encoding into uint32 by Big-Endian',
  `prefix` int(11) unsigned NOT NULL COMMENT 'IP 掩码，1～31',
  `networking_id` varchar(128) NOT NULL COMMENT '所属networking ID',
  `unit_id` varchar(128) DEFAULT NULL COMMENT '所属unit ID',
  `allocated` tinyint(1) NOT NULL DEFAULT '0' COMMENT '是否被分配，0为未分配，1为已分配',
  PRIMARY KEY (`ip_addr`,`prefix`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8;
/*!40101 SET character_set_client = @saved_cs_client */;

--
-- Dumping data for table `tb_ip`
--

LOCK TABLES `tb_ip` WRITE;
/*!40000 ALTER TABLE `tb_ip` DISABLE KEYS */;
/*!40000 ALTER TABLE `tb_ip` ENABLE KEYS */;
UNLOCK TABLES;

--
-- Table structure for table `tb_lun`
--

DROP TABLE IF EXISTS `tb_lun`;
/*!40101 SET @saved_cs_client     = @@character_set_client */;
/*!40101 SET character_set_client = utf8 */;
CREATE TABLE `tb_lun` (
  `id` varchar(128) NOT NULL,
  `storage_lun_id` int(11) NOT NULL COMMENT '在存储系统上的LUN ID',
  `name` varchar(128) NOT NULL COMMENT 'LUN 名称',
  `storage_system_id` varchar(128) NOT NULL COMMENT '所属存储系统ID',
  `raid_group_id` varchar(128) NOT NULL COMMENT '所属Raid Group ID(平台分配的ID)',
  `size` int(11) NOT NULL COMMENT 'LUN 容量大小，单位为M',
  `vg_name` varchar(128) DEFAULT NULL COMMENT '所属Volume Group 名称',
  `mapping_hostname` varchar(128) DEFAULT NULL COMMENT 'LUN 映射主机名称',
  `host_lun_id` int(11) DEFAULT NULL COMMENT '在映射主机上的LUN ID',
  `created_at` datetime NOT NULL,
  PRIMARY KEY (`id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8;
/*!40101 SET character_set_client = @saved_cs_client */;

--
-- Dumping data for table `tb_lun`
--

LOCK TABLES `tb_lun` WRITE;
/*!40000 ALTER TABLE `tb_lun` DISABLE KEYS */;
/*!40000 ALTER TABLE `tb_lun` ENABLE KEYS */;
UNLOCK TABLES;

--
-- Table structure for table `tb_networking`
--

DROP TABLE IF EXISTS `tb_networking`;
/*!40101 SET @saved_cs_client     = @@character_set_client */;
/*!40101 SET character_set_client = utf8 */;
CREATE TABLE `tb_networking` (
  `id` varchar(128) NOT NULL,
  `type` varchar(64) NOT NULL COMMENT '网络类型：\ninternal_access_networking	内部业务网	\nexternal_access_networking	外部接入网',
  `gateway` varchar(64) NOT NULL COMMENT '网关IP',
  `enabled` tinyint(1) unsigned DEFAULT '1' COMMENT '0：停用，1：启用',
  PRIMARY KEY (`id`),
  UNIQUE KEY `id_UNIQUE` (`id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8;
/*!40101 SET character_set_client = @saved_cs_client */;

--
-- Dumping data for table `tb_networking`
--

LOCK TABLES `tb_networking` WRITE;
/*!40000 ALTER TABLE `tb_networking` DISABLE KEYS */;
/*!40000 ALTER TABLE `tb_networking` ENABLE KEYS */;
UNLOCK TABLES;

--
-- Table structure for table `tb_node`
--

DROP TABLE IF EXISTS `tb_node`;
/*!40101 SET @saved_cs_client     = @@character_set_client */;
/*!40101 SET character_set_client = utf8 */;
CREATE TABLE `tb_node` (
  `id` varchar(128) NOT NULL COMMENT 'node ID',
  `name` varchar(128) NOT NULL COMMENT 'node 名称',
  `cluster_id` varchar(128) NOT NULL COMMENT '所属 Cluster',
  `engine_id` varchar(128) DEFAULT NULL COMMENT 'docker engine id',
  `admin_ip` varchar(128) NOT NULL COMMENT '内网网卡 IP，UINT32',
  `room` varchar(128) DEFAULT NULL,
  `seat` varchar(128) DEFAULT NULL,
  `max_container` int(11) NOT NULL,
  `status` int(4) unsigned NOT NULL DEFAULT '0' COMMENT '物理机状态\n0	准备入库			import\n1	初始化安装中		installing\n2	初始化安装成功	installed\n3	初始化安装失败	installfailed\n4	测试中			testing\n5	测试失败			failedtest\n6	启用				enable\n7	停用				disable\n',
  `register_at` datetime DEFAULT NULL COMMENT '注册时间',
  `deregister_at` datetime DEFAULT NULL COMMENT '注销时间',
  PRIMARY KEY (`id`),
  UNIQUE KEY `id_UNIQUE` (`id`),
  UNIQUE KEY `name_UNIQUE` (`name`),
  UNIQUE KEY `admin_ip_UNIQUE` (`admin_ip`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8;
/*!40101 SET character_set_client = @saved_cs_client */;

--
-- Dumping data for table `tb_node`
--

LOCK TABLES `tb_node` WRITE;
/*!40000 ALTER TABLE `tb_node` DISABLE KEYS */;
/*!40000 ALTER TABLE `tb_node` ENABLE KEYS */;
UNLOCK TABLES;

--
-- Table structure for table `tb_port`
--

DROP TABLE IF EXISTS `tb_port`;
/*!40101 SET @saved_cs_client     = @@character_set_client */;
/*!40101 SET character_set_client = utf8 */;
CREATE TABLE `tb_port` (
  `port` int(11) unsigned NOT NULL COMMENT '端口号，10000+，分配给容器使用，容器的端口是唯一的。',
  `name` varchar(128) DEFAULT NULL,
  `unit_id` varchar(128) DEFAULT NULL,
  `proto` varchar(45) DEFAULT NULL COMMENT '协议类型 tcp ／ udp',
  `allocated` tinyint(1) unsigned NOT NULL DEFAULT '0' COMMENT '是否被分配，0：未分配，1：已分配；',
  PRIMARY KEY (`port`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8;
/*!40101 SET character_set_client = @saved_cs_client */;

--
-- Dumping data for table `tb_port`
--

LOCK TABLES `tb_port` WRITE;
/*!40000 ALTER TABLE `tb_port` DISABLE KEYS */;
/*!40000 ALTER TABLE `tb_port` ENABLE KEYS */;
UNLOCK TABLES;

--
-- Table structure for table `tb_raid_group`
--

DROP TABLE IF EXISTS `tb_raid_group`;
/*!40101 SET @saved_cs_client     = @@character_set_client */;
/*!40101 SET character_set_client = utf8 */;
CREATE TABLE `tb_raid_group` (
  `id` varchar(128) NOT NULL COMMENT '存储RAID ID',
  `storage_rg_id` int(11) NOT NULL COMMENT '在存储系统上的Raid group ID',
  `storage_system_id` varchar(128) NOT NULL COMMENT 'RAID GROUP 所属存储系统ID',
  `enabled` tinyint(1) unsigned NOT NULL DEFAULT '1' COMMENT '是否启用，0：停用，1：启用。',
  PRIMARY KEY (`id`),
  UNIQUE KEY `id_UNIQUE` (`id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8;
/*!40101 SET character_set_client = @saved_cs_client */;

--
-- Dumping data for table `tb_raid_group`
--

LOCK TABLES `tb_raid_group` WRITE;
/*!40000 ALTER TABLE `tb_raid_group` DISABLE KEYS */;
INSERT INTO `tb_raid_group` VALUES ('raidGroupId001',1,'raidGroupStorageID001',1),('raidGroupId002',2,'raidGroupStorageID001',1);
/*!40000 ALTER TABLE `tb_raid_group` ENABLE KEYS */;
UNLOCK TABLES;

--
-- Table structure for table `tb_service`
--

DROP TABLE IF EXISTS `tb_service`;
/*!40101 SET @saved_cs_client     = @@character_set_client */;
/*!40101 SET character_set_client = utf8 */;
CREATE TABLE `tb_service` (
  `id` varchar(128) NOT NULL COMMENT '主键',
  `name` varchar(128) NOT NULL COMMENT 'Service 名称',
  `description` text NOT NULL COMMENT 'Service描述信息',
  `architecture` varchar(128) NOT NULL COMMENT 'service  结构:SW-nP-M-nSB-nSLV',
  `auto_healing` tinyint(1) unsigned DEFAULT '0' COMMENT '服务自动愈合，0:不启用，1：启用',
  `auto_scaling` tinyint(1) unsigned DEFAULT '0' COMMENT '自动扩缩，0：不启用，1：启用',
  `high_available` tinyint(1) unsigned DEFAULT '0' COMMENT '高可用，0：不启用，1：启用',
  `backup_max_size` bigint(128) unsigned DEFAULT NULL COMMENT '备份文件总大小，单位：MB',
  `backup_files_retention` bigint(128) DEFAULT NULL COMMENT '文件保存时间，time.Unix()',
  `status` tinyint(4) unsigned DEFAULT NULL COMMENT '管理状态\n0	已分配\n1	创建中\n2	启动中\n3	停止中\n4	删除中\n5	备份中\n6	还原中\n99	无任务',
  `created_at` datetime NOT NULL COMMENT '服务创建日期',
  `finished_at` datetime DEFAULT NULL COMMENT '服务创建完成日期',
  PRIMARY KEY (`id`),
  UNIQUE KEY `id_UNIQUE` (`id`),
  UNIQUE KEY `name_UNIQUE` (`name`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8;
/*!40101 SET character_set_client = @saved_cs_client */;

--
-- Dumping data for table `tb_service`
--

LOCK TABLES `tb_service` WRITE;
/*!40000 ALTER TABLE `tb_service` DISABLE KEYS */;
/*!40000 ALTER TABLE `tb_service` ENABLE KEYS */;
UNLOCK TABLES;

--
-- Table structure for table `tb_storage_HITACHI`
--

DROP TABLE IF EXISTS `tb_storage_HITACHI`;
/*!40101 SET @saved_cs_client     = @@character_set_client */;
/*!40101 SET character_set_client = utf8 */;
CREATE TABLE `tb_storage_HITACHI` (
  `id` varchar(128) NOT NULL COMMENT 'storage_system_ID',
  `vendor` varchar(128) NOT NULL COMMENT '厂商，HUAWEI / HITACHI',
  `admin_unit` varchar(128) NOT NULL COMMENT '管理域名称，HDS专有',
  `lun_start` int(11) unsigned NOT NULL COMMENT '起始位，HDS专有',
  `lun_end` int(11) unsigned NOT NULL COMMENT '结束位，HDS专有',
  `hlu_start` int(11) unsigned NOT NULL COMMENT 'host_lun_start ',
  `hlu_end` int(11) unsigned NOT NULL COMMENT 'host_un_end',
  PRIMARY KEY (`id`),
  UNIQUE KEY `id_UNIQUE` (`id`),
  UNIQUE KEY `admin_unit_UNIQUE` (`admin_unit`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8;
/*!40101 SET character_set_client = @saved_cs_client */;

--
-- Dumping data for table `tb_storage_HITACHI`
--

LOCK TABLES `tb_storage_HITACHI` WRITE;
/*!40000 ALTER TABLE `tb_storage_HITACHI` DISABLE KEYS */;
INSERT INTO `tb_storage_HITACHI` VALUES ('HitachiStorageId001','HitachiStorageVendor001','HitachiStorageAdminUnit001',1,5,11,55),('HitachiStorageId002','HitachiStorageVendor002','HitachiStorageAdminUnit002',1,5,11,55);
/*!40000 ALTER TABLE `tb_storage_HITACHI` ENABLE KEYS */;
UNLOCK TABLES;

--
-- Table structure for table `tb_storage_HUAWEI`
--

DROP TABLE IF EXISTS `tb_storage_HUAWEI`;
/*!40101 SET @saved_cs_client     = @@character_set_client */;
/*!40101 SET character_set_client = utf8 */;
CREATE TABLE `tb_storage_HUAWEI` (
  `id` varchar(128) NOT NULL COMMENT 'storage_system_ID',
  `vendor` varchar(128) NOT NULL COMMENT '厂商，huawei / HDS',
  `ip_addr` varchar(45) NOT NULL COMMENT '管理IP，huawei专有',
  `username` varchar(45) NOT NULL COMMENT '用户名，huawei专有',
  `password` varchar(45) NOT NULL COMMENT '密码，huawei专有',
  `hlu_start` int(11) unsigned NOT NULL COMMENT 'host_lun_start ',
  `hlu_end` int(11) unsigned NOT NULL COMMENT 'host_un_end',
  PRIMARY KEY (`id`),
  UNIQUE KEY `id_UNIQUE` (`id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8;
/*!40101 SET character_set_client = @saved_cs_client */;

--
-- Dumping data for table `tb_storage_HUAWEI`
--

LOCK TABLES `tb_storage_HUAWEI` WRITE;
/*!40000 ALTER TABLE `tb_storage_HUAWEI` DISABLE KEYS */;
INSERT INTO `tb_storage_HUAWEI` VALUES ('HuaweiStorageID001','HuaweiStorageVendor001','146.240.104.1','HuaweiStorageUsername001','HuaweiStoragePassword001',1,5),('HuaweiStorageID002','HuaweiStorageVendor002','146.240.104.1','HuaweiStorageUsername002','HuaweiStoragePassword002',1,5);
/*!40000 ALTER TABLE `tb_storage_HUAWEI` ENABLE KEYS */;
UNLOCK TABLES;

--
-- Table structure for table `tb_system_config`
--

DROP TABLE IF EXISTS `tb_system_config`;
/*!40101 SET @saved_cs_client     = @@character_set_client */;
/*!40101 SET character_set_client = utf8 */;
CREATE TABLE `tb_system_config` (
  `id` int(10) unsigned NOT NULL AUTO_INCREMENT COMMENT ' 主键，自增',
  `consul_IPs` varchar(45) NOT NULL COMMENT 'Consul Server集群IP地址',
  `consul_port` int(10) unsigned NOT NULL COMMENT 'Consul Server HTTP Port',
  `consul_dc` varchar(45) DEFAULT NULL COMMENT 'Consul DataCenter',
  `consul_token` varchar(45) DEFAULT NULL COMMENT 'Consul Token',
  `consul_wait_time` int(10) DEFAULT NULL,
  `docker_port` int(10) unsigned NOT NULL COMMENT 'Agent HTTP Port',
  `plugin_port` int(6) DEFAULT NULL,
  `horus_agent_port` int(11) DEFAULT NULL,
  `horus_server_ip` varchar(45) DEFAULT NULL COMMENT 'Horus distribution system HTTP IP',
  `horus_server_port` int(10) unsigned DEFAULT NULL COMMENT 'Horus distribution system HTTP Port',
  `horus_event_ip` varchar(45) DEFAULT NULL,
  `horus_event_port` int(10) unsigned DEFAULT NULL,
  `retry` tinyint(4) DEFAULT NULL COMMENT '失败重试次数',
  `registry_address` varchar(45) DEFAULT NULL,
  `registry_port` int(6) DEFAULT NULL,
  `registry_username` varchar(45) DEFAULT NULL,
  `registry_password` varchar(45) DEFAULT NULL,
  `registry_email` varchar(45) DEFAULT NULL,
  `registry_token` varchar(4096) DEFAULT NULL,
  `registry_domain` varchar(45) DEFAULT NULL,
  `registry_ca_crt` varchar(4096) DEFAULT NULL COMMENT 'registry证书文件内容',
  `registry_os_username` varchar(45) DEFAULT NULL,
  `registry_os_password` varchar(45) DEFAULT NULL,
  `pkg_name` varchar(45) DEFAULT NULL COMMENT '物理机初始化包名称',
  `source_dir` varchar(128) NOT NULL COMMENT '物理机初始化包绝对路径',
  `destination_dir` varchar(128) NOT NULL COMMENT '物理机初始化包远程目标目录',
  `script_name` varchar(45) NOT NULL COMMENT '物理机初始化脚本名',
  `ca_crt_name` varchar(45) NOT NULL COMMENT '证书文件名称',
  `mon_username` varchar(128) NOT NULL,
  `mon_password` varchar(128) NOT NULL,
  PRIMARY KEY (`id`)
) ENGINE=InnoDB AUTO_INCREMENT=7 DEFAULT CHARSET=utf8;
/*!40101 SET character_set_client = @saved_cs_client */;

--
-- Dumping data for table `tb_system_config`
--

LOCK TABLES `tb_system_config` WRITE;
/*!40000 ALTER TABLE `tb_system_config` DISABLE KEYS */;
INSERT INTO `tb_system_config` VALUES (6,'192.168.2.131,192.168.2.132,192.168.2.133',8500,'dc1','',15,2375,0,8123,'192.168.2.123',8000,'192.168.2.123',8484,0,'192.168.2.120',8080,'docker','unionpay','@unionpay.com','','registry.dbaas.me','-----BEGIN CERTIFICATE-----\nMIIFRjCCAy4CCQDm8L9Hil8uhzANBgkqhkiG9w0BAQUFADBlMQswCQYDVQQGEwJD\nTjERMA8GA1UECAwIU2hhbmdoYWkxDzANBgNVBAcMBlB1RG9uZzEWMBQGA1UECgwN\nQ2hpbmFVbmlvblBheTEaMBgGA1UEAwwRcmVnaXN0cnkuZGJhYXMubWUwHhcNMTYw\nNTE3MDMyNDE1WhcNMjYwNTE1MDMyNDE1WjBlMQswCQYDVQQGEwJDTjERMA8GA1UE\nCAwIU2hhbmdoYWkxDzANBgNVBAcMBlB1RG9uZzEWMBQGA1UECgwNQ2hpbmFVbmlv\nblBheTEaMBgGA1UEAwwRcmVnaXN0cnkuZGJhYXMubWUwggIiMA0GCSqGSIb3DQEB\nAQUAA4ICDwAwggIKAoICAQDOFSU46PohiMcRGYqm7cl6GjShOApQX3a+dIrdeCAt\nkVPHtBfBu8VC6/r4R/PBVU5ffxJeT8P2yyIgp+l9pOLOMZNAjteJY8G3+vkNltx6\nm7knl7Eack9W/Ki1XzREz0hYwT4hCOLgHnascmJvUn0AgjPlBHJGQFjIEqLJXuKL\ncys+BAWWN0/RFJpaSBbhv/Du5qF/wHJVdihx1gL4yL4Mzh5dY4Wq8y7V40DvSPeT\n2AaPJZ42EpPLBLDKS51CDse6v70vjV0yeyCwQvc4YHuP8BktRZWvm32D65bCS/EN\nZbTbXEkdr230u4RR0U+cIXOZlsK4+308nLf604BZFCLszVfFXApg1lKVeVIH0EJg\nirCGl0TPQp94qX9StLKFBF8YDnp3D9JrQQi7RCDOPP83NgIVYbzTgsA0fXfYDUqo\n3fNuMWJoHfFYrlSwqN62Qr1QZBANpNQaHbM0CtbbLhZ51r6oJE0YPT+YJDgAhe4K\nZ0hw9fWbbGHDQQtTKauzdVeAHHTroXMkHXZG3cxw3g63jKKHHQGllQNIp/lEZXKW\nHsVS+pEgXRTY/fRHMwqc72xsFiVslqTqJ7YisUjAdv9pOGhF90yqgfNxWLtdGPwR\nTs24tLPOdRRpN82OVS0LmGTx3mGL83ITZQaHaP4v+SxeBn7VYU5CY6vSNhfINXGc\nPwIDAQABMA0GCSqGSIb3DQEBBQUAA4ICAQBFnxk5oxqVlCfFoEIZx0SopCEIzuWU\n5cEShob+L4VDNlJ/JB2PsgxY1YWxTOgblisVMfvjjilIMePVe82NeJ9kg+Eut6WA\nyP3IHTgQnwbptiMys3ZLnZk3uohn0yd1I8QjE/bBtZtBqTw/z10UPoT1KI1TyRt2\nk45x4FPaTZntrtM5uniKyG6Ng3uRuOu+krkZlWgBAXFYdeyBGco+bBkFD9HC7+oc\npCNA4P+s2EOOhCHHG2oyWgQ2yZqYVX2RxHLZWA6QSNL6hoEJvqz9vCBC/JsaBER7\nSNzGhRq7ZVZNaERONcTZuLnDcGtKWwagPvnv8CoTw+S3UqggcT0L2l0LN74q7We5\n8PC4ip9kAO93/Pfr5VYwtrS6JMVCOOfeEDDFBcg+c63hkQ/JnkF7XmKCOfOjkBJY\nCsDUgAhdqVfZSR7AbxhpD42JGFhcgRWi+60/7ecMTSUSHXobNnlQ8VuDW+LgMyw5\nccL9GaKXiLAX1csqcRPf1bN4zbMkW1DMdJPLLxTLm52HAiFCP3C0BiDLBpopi3BV\nqt2g0gdTVHtNFBGTHpWQP9HUkhCVBTfojqnLSMUEtwEaMT6/HrEIUpVHKajlufSd\nY9JHjSgfqFD4g604wJPgwE0/52HQGh8gGWND1FgcfpPZsytLasV5VsxFVFH0TAeq\nEk9p+Ir1XAXsfg==\n-----END CERTIFICATE-----','root','root','','./script/node-init','/tmp','node-init.sh','registery-ca.crt','mon','123.com');
/*!40000 ALTER TABLE `tb_system_config` ENABLE KEYS */;
UNLOCK TABLES;

--
-- Table structure for table `tb_task`
--

DROP TABLE IF EXISTS `tb_task`;
/*!40101 SET @saved_cs_client     = @@character_set_client */;
/*!40101 SET character_set_client = utf8 */;
CREATE TABLE `tb_task` (
  `id` varchar(128) NOT NULL,
  `related` varchar(128) NOT NULL COMMENT '关联表名称或者对象。',
  `link_to` varchar(128) DEFAULT NULL COMMENT '关联ID',
  `description` varchar(128) DEFAULT NULL,
  `labels` varchar(512) DEFAULT NULL,
  `errors` varchar(128) DEFAULT NULL,
  `timeout` int(11) unsigned DEFAULT NULL,
  `status` tinyint(4) unsigned DEFAULT '0' COMMENT '任务状态\n0	创建任务				create\n1	执行中				running		\n2	任务中止,未完成		stop\n3	任务未执行，被取消		cancel\n4	任务完成				done\n5	任务超时				timeout\n6	任务失败				faile',
  `created_at` datetime NOT NULL,
  `finished_at` datetime DEFAULT NULL,
  PRIMARY KEY (`id`),
  UNIQUE KEY `id_UNIQUE` (`id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8;
/*!40101 SET character_set_client = @saved_cs_client */;

--
-- Dumping data for table `tb_task`
--

LOCK TABLES `tb_task` WRITE;
/*!40000 ALTER TABLE `tb_task` DISABLE KEYS */;
/*!40000 ALTER TABLE `tb_task` ENABLE KEYS */;
UNLOCK TABLES;

--
-- Table structure for table `tb_unit`
--

DROP TABLE IF EXISTS `tb_unit`;
/*!40101 SET @saved_cs_client     = @@character_set_client */;
/*!40101 SET character_set_client = utf8 */;
CREATE TABLE `tb_unit` (
  `id` varchar(128) NOT NULL COMMENT 'unit ID',
  `name` varchar(128) NOT NULL COMMENT 'unit 名称，命名规则为<unit_id_8bit>_<service_name>',
  `type` varchar(45) NOT NULL COMMENT 'unit 类型， switch_manager / upproxy / upsql ',
  `service_id` varchar(128) NOT NULL COMMENT ' 所属 Service ID',
  `image_id` varchar(128) NOT NULL,
  `image_name` varchar(128) NOT NULL COMMENT '镜像名称，命名规则为<software_name>_<version>',
  `network_mode` varchar(45) DEFAULT NULL,
  `node_id` varchar(128) DEFAULT NULL COMMENT '所在主机ID',
  `container_id` varchar(128) DEFAULT NULL COMMENT 'docker 生成的ID',
  `unit_config_id` varchar(128) NOT NULL COMMENT '配置文件ID',
  `check_interval` int(11) unsigned DEFAULT NULL COMMENT '服务检查间隔时间,单位为秒',
  `status` int(11) unsigned NOT NULL COMMENT '管理状态\n0	已分配\n1	创建中\n2	启动中\n3	停止中\n4	迁移中\n5	重建中\n6	删除中\n7	备份中\n8	还原中\n99	无任务',
  `created_at` datetime NOT NULL,
  PRIMARY KEY (`id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8;
/*!40101 SET character_set_client = @saved_cs_client */;

--
-- Dumping data for table `tb_unit`
--

LOCK TABLES `tb_unit` WRITE;
/*!40000 ALTER TABLE `tb_unit` DISABLE KEYS */;
/*!40000 ALTER TABLE `tb_unit` ENABLE KEYS */;
UNLOCK TABLES;

--
-- Table structure for table `tb_unit_config`
--

DROP TABLE IF EXISTS `tb_unit_config`;
/*!40101 SET @saved_cs_client     = @@character_set_client */;
/*!40101 SET character_set_client = utf8 */;
CREATE TABLE `tb_unit_config` (
  `id` varchar(128) NOT NULL,
  `image_id` varchar(128) NOT NULL COMMENT '镜像软件ID',
  `version` int(11) NOT NULL COMMENT '版本号\n从0 开始，更新一次＋1',
  `parent_id` varchar(128) DEFAULT NULL COMMENT '前一次的配置文件ID',
  `content` longtext NOT NULL COMMENT '配置文件内容',
  `config_file_path` varchar(128) NOT NULL,
  `config_key_sets` varchar(4096) DEFAULT NULL COMMENT '配置文件中可修改key的集合\n[key1,key2]',
  `created_at` datetime NOT NULL,
  PRIMARY KEY (`id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8;
/*!40101 SET character_set_client = @saved_cs_client */;

--
-- Dumping data for table `tb_unit_config`
--

LOCK TABLES `tb_unit_config` WRITE;
/*!40000 ALTER TABLE `tb_unit_config` DISABLE KEYS */;
/*!40000 ALTER TABLE `tb_unit_config` ENABLE KEYS */;
UNLOCK TABLES;

--
-- Table structure for table `tb_users`
--

DROP TABLE IF EXISTS `tb_users`;
/*!40101 SET @saved_cs_client     = @@character_set_client */;
/*!40101 SET character_set_client = utf8 */;
CREATE TABLE `tb_users` (
  `id` varchar(128) NOT NULL,
  `service_id` varchar(128) NOT NULL,
  `type` varchar(45) NOT NULL COMMENT 'upsql or up proxy',
  `username` varchar(128) NOT NULL,
  `password` varchar(128) DEFAULT NULL,
  `role` varchar(45) NOT NULL COMMENT 'cup_db,db,op,or',
  `created_at` datetime NOT NULL,
  PRIMARY KEY (`id`),
  UNIQUE KEY `id_UNIQUE` (`id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8;
/*!40101 SET character_set_client = @saved_cs_client */;

--
-- Dumping data for table `tb_users`
--

LOCK TABLES `tb_users` WRITE;
/*!40000 ALTER TABLE `tb_users` DISABLE KEYS */;
INSERT INTO `tb_users` VALUES ('userId001','userServiceId001','userType001','userName001','userPassword001','userRole001','2016-05-19 13:25:07'),('userId002','userServiceId002','userType002','userName002','userPassword002','userRole002','2016-05-19 13:25:07');
/*!40000 ALTER TABLE `tb_users` ENABLE KEYS */;
UNLOCK TABLES;

--
-- Table structure for table `tb_volumes`
--

DROP TABLE IF EXISTS `tb_volumes`;
/*!40101 SET @saved_cs_client     = @@character_set_client */;
/*!40101 SET character_set_client = utf8 */;
CREATE TABLE `tb_volumes` (
  `id` varchar(128) NOT NULL,
  `name` varchar(128) NOT NULL,
  `size` int(11) NOT NULL COMMENT 'volume 容量大小，单位byte',
  `VGname` varchar(128) NOT NULL,
  `driver` varchar(45) NOT NULL,
  `fstype` varchar(45) NOT NULL,
  `unit_id` varchar(128) NOT NULL COMMENT '所属Unit ID',
  PRIMARY KEY (`id`,`name`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8;
/*!40101 SET character_set_client = @saved_cs_client */;

--
-- Dumping data for table `tb_volumes`
--

LOCK TABLES `tb_volumes` WRITE;
/*!40000 ALTER TABLE `tb_volumes` DISABLE KEYS */;
/*!40000 ALTER TABLE `tb_volumes` ENABLE KEYS */;
UNLOCK TABLES;
/*!40103 SET TIME_ZONE=@OLD_TIME_ZONE */;

/*!40101 SET SQL_MODE=@OLD_SQL_MODE */;
/*!40014 SET FOREIGN_KEY_CHECKS=@OLD_FOREIGN_KEY_CHECKS */;
/*!40014 SET UNIQUE_CHECKS=@OLD_UNIQUE_CHECKS */;
/*!40101 SET CHARACTER_SET_CLIENT=@OLD_CHARACTER_SET_CLIENT */;
/*!40101 SET CHARACTER_SET_RESULTS=@OLD_CHARACTER_SET_RESULTS */;
/*!40101 SET COLLATION_CONNECTION=@OLD_COLLATION_CONNECTION */;
/*!40111 SET SQL_NOTES=@OLD_SQL_NOTES */;

-- Dump completed on 2016-05-20 16:35:30
