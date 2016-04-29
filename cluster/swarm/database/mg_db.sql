-- MySQL dump 10.13  Distrib 5.6.22, for osx10.8 (x86_64)
--
-- Host: 192.168.16.31    Database: DBaaS
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
  `spec` varchar(45) NOT NULL COMMENT '触发规则描述，cron语法',
  `next` int(10) unsigned DEFAULT '0' COMMENT '下一次执行时间',
  `valid` datetime NOT NULL COMMENT '有效期限',
  `timeout` int(11) unsigned DEFAULT NULL COMMENT '执行备份的超时时长，time.Unix()值。',
  `backup_dir` varchar(128) NOT NULL COMMENT '备份目录',
  `max_size` int(11) unsigned DEFAULT NULL COMMENT '备份文件总大小，单位：MB',
  `retention` int(11) DEFAULT NULL COMMENT '文件保存时间，time.Unix()',
  `type` varchar(64) DEFAULT NULL COMMENT '备份类型\n全量／增量\nfull／incremental\n',
  `enabled` tinyint(1) unsigned DEFAULT '1' COMMENT '0:停用，1：启用',
  `create_at` datetime DEFAULT NULL COMMENT '创建时间',
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
  `name` varchar(64) NOT NULL COMMENT 'cluster name',
  `type` varchar(64) NOT NULL COMMENT 'upsql / upproxy / switch_manager',
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
INSERT INTO `tb_cluster` VALUES ('3f1ebb615c5d8ba700ba65c9ad43e2c3e6957ae9cdc34c2141f04f6499fe3392','cluster001','upsql','local','',100,0.8,'',1);
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
  `name` varchar(45) NOT NULL COMMENT '软件包名称',
  `size` int(11) DEFAULT NULL,
  `docker_image_id` varchar(128) DEFAULT NULL COMMENT 'docker image id',
  `version` varchar(45) NOT NULL COMMENT '软件版本号',
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
INSERT INTO `tb_image` VALUES ('f873d4a6d8a2680e0c28db155fba503cec90f078550f9cffddf8b5a4bd30bdfb','upsql',2401,'sha256:a81fb41a3808058bd177948c12ededdae03a5a65059c0185346acf266e09e279','5.6.19','9912c790423201473bb963c59893a3bb25f066d57011dc77984bd6e6d6b64d3d','null\n',1,'2016-04-28 22:58:45');
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
  `networking_id` varchar(128) NOT NULL COMMENT 'tb_network ID',
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
INSERT INTO `tb_ip` VALUES (3232239766,24,'cc9ba0acb7ac5783d49d93fd9f8605570a337696420e7343ad834feca0780e42','',0),(3232239767,24,'cc9ba0acb7ac5783d49d93fd9f8605570a337696420e7343ad834feca0780e42','',0),(3232239768,24,'cc9ba0acb7ac5783d49d93fd9f8605570a337696420e7343ad834feca0780e42','',0),(3232239769,24,'cc9ba0acb7ac5783d49d93fd9f8605570a337696420e7343ad834feca0780e42','',0),(3232239770,24,'cc9ba0acb7ac5783d49d93fd9f8605570a337696420e7343ad834feca0780e42','',0),(3232239771,24,'cc9ba0acb7ac5783d49d93fd9f8605570a337696420e7343ad834feca0780e42','',0),(3232239772,24,'cc9ba0acb7ac5783d49d93fd9f8605570a337696420e7343ad834feca0780e42','',0),(3232239773,24,'cc9ba0acb7ac5783d49d93fd9f8605570a337696420e7343ad834feca0780e42','',0),(3232239774,24,'cc9ba0acb7ac5783d49d93fd9f8605570a337696420e7343ad834feca0780e42','',0),(3232239775,24,'cc9ba0acb7ac5783d49d93fd9f8605570a337696420e7343ad834feca0780e42','',0);
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
  `unit_id` varchar(128) DEFAULT NULL COMMENT '所属UNIT ID',
  `mapping_hostname` varchar(45) DEFAULT NULL COMMENT 'LUN 映射主机名称',
  `host_lun_id` int(11) DEFAULT NULL COMMENT '在映射主机上的LUN ID',
  `create_at` datetime NOT NULL,
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
  `networking` varchar(45) NOT NULL COMMENT '网段,如192.168.1.0/24',
  `gateway` varchar(45) NOT NULL COMMENT '网关IP',
  `enabled` tinyint(1) unsigned DEFAULT '1' COMMENT '0：停用，1：启用',
  PRIMARY KEY (`id`),
  UNIQUE KEY `network_UNIQUE` (`networking`),
  UNIQUE KEY `id_UNIQUE` (`id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8;
/*!40101 SET character_set_client = @saved_cs_client */;

--
-- Dumping data for table `tb_networking`
--

LOCK TABLES `tb_networking` WRITE;
/*!40000 ALTER TABLE `tb_networking` DISABLE KEYS */;
INSERT INTO `tb_networking` VALUES ('cc9ba0acb7ac5783d49d93fd9f8605570a337696420e7343ad834feca0780e42','internal_access_networking','192.168.16.150','192.168.16.1',1);
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
  `name` varchar(64) NOT NULL COMMENT 'node 名称',
  `cluster_id` varchar(128) NOT NULL COMMENT '所属 Cluster',
  `engine_id` varchar(128) DEFAULT NULL COMMENT 'docker engine id',
  `admin_ip` varchar(128) NOT NULL COMMENT '内网网卡 IP，UINT32',
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
INSERT INTO `tb_node` VALUES ('7021fbf0f2112bb00a52c69a5e7c62bd28808e7a081ad0f36131e3047c2cb703','node001','3f1ebb615c5d8ba700ba65c9ad43e2c3e6957ae9cdc34c2141f04f6499fe3392','YK4Z:4H7U:67IH:KCWJ:4LBY:RNHO:Q2EY:PVEA:Q7NR:S7UB:WZEE:GLXY','192.168.16.41',4,6,'2016-04-29 02:18:42','0000-00-00 00:00:00');
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
  `name` varchar(45) DEFAULT NULL,
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
INSERT INTO `tb_port` VALUES (30000,'','','',0),(30001,'','','',0),(30002,'','','',0),(30003,'','','',0),(30004,'','','',0),(30005,'','','',0),(30006,'','','',0),(30007,'','','',0),(30008,'','','',0),(30009,'','','',0);
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
  `name` varchar(64) NOT NULL COMMENT 'Service 名称',
  `description` varchar(45) DEFAULT NULL COMMENT 'Service描述信息',
  `architecture` varchar(128) NOT NULL COMMENT 'service  结构:SW-nP-M-nSB-nSLV',
  `auto_healing` tinyint(1) unsigned DEFAULT '0' COMMENT '服务自动愈合，0:不启用，1：启用',
  `auto_scaling` tinyint(1) unsigned DEFAULT '0' COMMENT '自动扩缩，0：不启用，1：启用',
  `high_available` tinyint(1) unsigned DEFAULT '0' COMMENT '高可用，0：不启用，1：启用',
  `status` tinyint(4) unsigned DEFAULT NULL COMMENT '管理状态\n0	已分配\n1	创建中\n2	启动中\n3	停止中\n4	删除中\n5	备份中\n6	还原中\n99	无任务',
  `backup_strategy_id` varchar(128) DEFAULT NULL COMMENT '备份策略ID',
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
  `vendor` varchar(45) NOT NULL COMMENT '厂商，HUAWEI / HITACHI',
  `admin_unit` varchar(45) NOT NULL COMMENT '管理域名称，HDS专有',
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
  `vendor` varchar(45) NOT NULL COMMENT '厂商，huawei / HDS',
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
  `registry_username` varchar(45) DEFAULT NULL,
  `registry_password` varchar(45) DEFAULT NULL,
  `registry_email` varchar(45) DEFAULT NULL,
  `registry_token` varchar(4096) DEFAULT NULL,
  `registry_domain` varchar(45) DEFAULT NULL,
  `registry_address` varchar(45) DEFAULT NULL,
  `registry_port` int(6) DEFAULT NULL,
  `registry_ca_crt` varchar(4096) DEFAULT NULL COMMENT 'registry证书文件内容',
  `registry_os_username` varchar(45) DEFAULT NULL,
  `registry_os_password` varchar(45) DEFAULT NULL,
  `pkg_name` varchar(45) DEFAULT NULL COMMENT '物理机初始化包名称',
  `source_dir` varchar(128) NOT NULL COMMENT '物理机初始化包绝对路径',
  `destination_dir` varchar(128) NOT NULL COMMENT '物理机初始化包远程目标目录',
  `script_name` varchar(45) NOT NULL COMMENT '物理机初始化脚本名',
  `ca_crt_name` varchar(45) NOT NULL COMMENT '证书文件名称',
  PRIMARY KEY (`id`)
) ENGINE=InnoDB AUTO_INCREMENT=12 DEFAULT CHARSET=utf8;
/*!40101 SET character_set_client = @saved_cs_client */;

--
-- Dumping data for table `tb_system_config`
--

LOCK TABLES `tb_system_config` WRITE;
/*!40000 ALTER TABLE `tb_system_config` DISABLE KEYS */;
INSERT INTO `tb_system_config` VALUES (6,'192.168.16.30,192.168.16.31,192.168.16.32',8500,'dc1','',15,2375,0,8123,'192.168.16.21',8000,'192.168.16.21',8484,0,'docker','unionpay','@unionpay.com','','registry.dbaas.me','192.168.16.30',8080,'-----BEGIN CERTIFICATE-----\nMIIFRjCCAy4CCQDyr289BRlTZjANBgkqhkiG9w0BAQUFADBlMQswCQYDVQQGEwJD\nTjERMA8GA1UECAwIU2hhbmdoYWkxDzANBgNVBAcMBlB1RG9uZzEWMBQGA1UECgwN\nQ2hpbmFVbmlvblBheTEaMBgGA1UEAwwRcmVnaXN0cnkuZGJhYXMubWUwHhcNMTYw\nNDI3MTIzOTM0WhcNMjYwNDI1MTIzOTM0WjBlMQswCQYDVQQGEwJDTjERMA8GA1UE\nCAwIU2hhbmdoYWkxDzANBgNVBAcMBlB1RG9uZzEWMBQGA1UECgwNQ2hpbmFVbmlv\nblBheTEaMBgGA1UEAwwRcmVnaXN0cnkuZGJhYXMubWUwggIiMA0GCSqGSIb3DQEB\nAQUAA4ICDwAwggIKAoICAQC0BsnMDrig83BcFMUx0h3M0Dd04vs4bGbVXV+BrB3N\nPnW6F/ZqFIFssRU56B1EKhduf1f3Hk/0/6A9D4no6WeVkLW2aVEDI+EBiFq3RnBt\neyJuXgrCT2H2qCRUIL3WEMaet/0AH9LGZ7ywB304nHfAwizBSE9nkhoypF4lfmsl\nAgteN9Ff8noqgBeRyrITmZMaDfAEqhZkbc4z1b8dU0Op88WrzU6ZSWCWRBQ4mz4Z\nm6FEWZB8+0jQilkSUZIMPhDyuNIMGTsZ50ZQnG7mVIF35iWh+9bOZLVSAoY/SWTB\nNDB/Ps9IDR/A+3uwvrKk6BGw46yWnW2/mFi3GaA6VmHBPS9QjxkwQgkveaeijHsP\nQD5tD6F0UTC8ydXh5BqPyn0GYdz6DezGY+3AOVPO/pyzzsvy251TbYGLdAqh30gv\nf5CVGNOrf/M7BaiFaMuXyUovENPjk5hglmYxJpOHJYwZgkwOigLqSlM5yXrR2k5/\nwlEEUjU8qP1vAJ8klAHV7FRQFaXflAG0mLYLFFg1vFKNT183Mkp3u1oGAPut6/9s\nb153OPLx8g0F7g1vlgQTGbx1PwNf3m6nxn2QICGWcqF55mQUd4RCtLGTi0JsZiPr\nOdcLBak+ncATX9if5tumi4uJT8D2wkgH/+dPUw/H2BTeztCHd1Pp/ZT13KJHv/yN\nVwIDAQABMA0GCSqGSIb3DQEBBQUAA4ICAQCioocar6bPBYG2katy8wA2IX/YdOgU\ncwR7WkF35Vy+QciUaSBDyS7FWaE2mmgOPdP8cyC7xAiBkkN7SBTG4u7obZgqJxKE\n7PSEfT1FmhTzd9u01lqB1vCZf28mTWxmVRIauwumGbsXQoPivhKBYQrZQ/Ut/GkN\nPe374+uTGq5sMVP+/n+llQVpI8pX32tW+s6IUINpmqciTCGbJVKASEM08WKe2bm4\nunMqaET9D1SJgwMIndIYlZcu1g91iKGLOuw9OzIfa+IrNVcbiT/qU4rM7etXijYF\nN73arjiYcfD5CQI9lUqrot7cNbhzN3Te35SBprINpV5WkcgPPK1bMDPQWDDESQol\nh52tUSFFfcrDOpK8tQ5Fo70V/SPvNTWmgwt7PVEotWx0ctl48T2ijAYxEoWCR2/4\nFgqf23TgdWYat4565JFbBSGin+G6hpjAGUgrcJTNnIPJoAcwl3aB5M4JhJQ3H/j1\nbECt/+NYT3Cn+x55fxEqyvD1erUUbveMnqGH0MS2ww3NXShk0J1TDAVP74QYqONH\n7kdV70Lg1wudJ0uOfHmetfGg5LuxRhZa6R3o0CZhYnS3kSXtZsSLOqfvIlDWnXHQ\nWOeAPfxK8jb5AX7eDbx8oel4aXMAmaVDGckHpcYO4UDhGJfH7l0aBn+Ge2Bwn2dK\nkxDuYr6T0rCbiA==\n-----END CERTIFICATE-----','root','root','','./script/node-init','/tmp','node-init.sh','registery-ca.crt');
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
  `description` varchar(45) DEFAULT NULL,
  `labels` varchar(512) DEFAULT NULL,
  `errors` varchar(128) DEFAULT NULL,
  `timeout` int(11) unsigned DEFAULT NULL,
  `status` tinyint(4) unsigned DEFAULT '0' COMMENT '任务状态\n0	创建任务				create\n1	执行中				running		\n2	任务中止,未完成		stop\n3	任务未执行，被取消		cancel\n4	任务完成				done\n5	任务超时				timeout\n6	任务失败				faile',
  `create_at` datetime NOT NULL,
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
INSERT INTO `tb_task` VALUES ('3e1fabfb6110ea5c9c79d8f206ed510b958e7a14a13969cc87fac336e1589b91','node','7021fbf0f2112bb00a52c69a5e7c62bd28808e7a081ad0f36131e3047c2cb703','import node','','',0,4,'2016-04-29 02:15:22','2016-04-29 02:18:42');
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
  `name` varchar(45) NOT NULL COMMENT 'unit 名称，命名规则为<unit_id_8bit>_<service_name>',
  `type` varchar(45) NOT NULL COMMENT 'unit 类型， switch_manager / upproxy / upsql ',
  `service_id` varchar(128) NOT NULL COMMENT ' 所属 Service ID',
  `image_id` varchar(128) NOT NULL,
  `image_name` varchar(45) NOT NULL COMMENT '镜像名称，命名规则为<software_name>_<version>',
  `network_mode` varchar(45) DEFAULT NULL,
  `node_id` varchar(45) DEFAULT NULL COMMENT '所在主机ID',
  `container_id` varchar(128) DEFAULT NULL COMMENT 'docker 生成的ID',
  `unit_config_id` varchar(128) NOT NULL COMMENT '配置文件ID',
  `check_interval` int(10) unsigned DEFAULT NULL COMMENT '服务检查间隔时间,单位为秒',
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
INSERT INTO `tb_unit_config` VALUES ('9912c790423201473bb963c59893a3bb25f066d57011dc77984bd6e6d6b64d3d','sha256:a81fb41a3808058bd177948c12ededdae03a5a65059c0185346acf266e09e279',0,'','','/DBAASDAT/my.cnf',NULL,'2016-04-28 22:58:45'),('df2f52da553c4157b2415fe05b6c85bd6e10231966f1bca58d5a8da9dbc061fb','sha256:8bc1b71fac2a61388dbd4cc4ea4b704b24c605ef841303b0ed1116d635b765c1',0,'','##################UpSQL 5.6.19##################[mysqld]bind-address =  <ip_addr>port = <port>socket = /DBAASDAT/upsql.sockserver-id = <server_id>character_set_server = gbkmax_connect_errors = 50000max_connections = 5000max_user_connections = 0skip-name-resolveskip_external_locking = ONmax_allowed_packet = 16Msort_buffer_size = 2Mjoin_buffer_size = 128Kuser = upsqltmpdir = /DBAASDATdatadir = /DBAASDATlog-bin = /DBAASLOG/BIN/<unit_id>-<ins_name>-binloglog_bin_trust_function_creators = ONsync_binlog = 1expire_logs_days = 0key_buffer_size = 160Mbinlog_cache_size = 1Mbinlog_format = rowlower_case_table_names = 1max_binlog_size = 1Gconnect_timeout = 60interactive_timeout = 31536000wait_timeout = 31536000net_read_timeout = 30net_write_timeout = 60optimizer_switch = \'mrr=on,mrr_cost_based=off\'open_files_limit = 10240explicit_defaults_for_timestamp = trueinnodb_open_files = 1024innodb_data_file_path=ibdata1:12M:autoextendinnodb_buffer_pool_size = 2Ginnodb_buffer_pool_instances = 8innodb_log_buffer_size = 128Minnodb_log_file_size = 512Minnodb_log_files_in_group = 7innodb_log_group_home_dir = /DBAASLOG/REDinnodb_max_dirty_pages_pct = 30innodb_flush_method = O_DIRECTinnodb_flush_log_at_trx_commit = 1innodb_thread_concurrency = 16innodb_read_io_threads = 4innodb_write_io_threads = 4innodb_lock_wait_timeout = 60innodb_rollback_on_timeout = oninnodb_file_per_table = 1innodb_stats_sample_pages = 1innodb_purge_threads = 1innodb_stats_on_metadata = OFFinnodb_support_xa = 1innodb_doublewrite = 1innodb_checksums = 1innodb_io_capacity = 500innodb_purge_threads = 8innodb_purge_batch_size = 500innodb_stats_persistent_sample_pages = 10plugin_dir = /usr/local/mysql/lib/pluginplugin_load = \"rpl_semi_sync_master=semisync_master.so;rpl_semi_sync_slave=semisync_slave.so;upsql_auth=upsql_auth.so\"loose_rpl_semi_sync_master_enabled = 1loose_rpl_semi_sync_slave_enabled = 1##[DBPM variables]upsql_auth_dbpm_mainip=144.7.32.31upsql_auth_dbpm_bkupip=144.7.34.31upsql_auth_dbpm_mainport=20010upsql_auth_dbpm_bkupport=20010upsql_auth_update_timeslice=3600upsql_auth_dbpm_serverid=upsqlupsql_auth_dbpm_tmtime=2upsql_ee_cheat_iplist=##[Replication variables]gtid-mode = onenforce-gtid-consistency = onlog-slave-updates = onbinlog_checksum = CRC32binlog_row_image = minimalslave_sql_verify_checksum = onslave_parallel_workers = 5master_verify_checksum  =   ONslave_sql_verify_checksum = ONmaster_info_repository=TABLErelay_log_info_repository=TABLEreplicate-ignore-db=dbaas_check##[Replication variables for Master]rpl_semi_sync_master_enabled = onauto_increment_incrementauto_increment_offsetrpl_semi_sync_master_timeout = 10000rpl_semi_sync_master_wait_no_slave = onrpl_semi_sync_master_trace_level = 32##[Replication variables for Slave]rpl_semi_sync_slave_enabled = onrpl_semi_sync_slave_trace_level = 32slave_net_timeout = 10relay_log_recovery = onlog_slave_updates = onmax_relay_log_size = 1Grelay_log = /DBAASLOG/REL/<unit_id>-<ins_name>-relayrelay_log_purge = on[mysqldump]quickmax_allowed_packet = 16M[myisamchk]key_buffer_size = 20Msort_buffer_size = 2Mread_buffer = 2Mwrite_buffer = 2M[mysqlhotcopy]interactive-timeout','/DBAASDAT/my.cnf',NULL,'2016-04-15 10:15:11');
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
  `service_id` varchar(45) NOT NULL,
  `type` varchar(45) NOT NULL COMMENT 'upsql or up proxy',
  `username` varchar(45) NOT NULL,
  `password` varchar(45) DEFAULT NULL,
  `role` varchar(45) NOT NULL COMMENT 'cup_db,db,op,or',
  `created_at` varchar(45) NOT NULL,
  PRIMARY KEY (`id`),
  UNIQUE KEY `id_UNIQUE` (`id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8;
/*!40101 SET character_set_client = @saved_cs_client */;

--
-- Dumping data for table `tb_users`
--

LOCK TABLES `tb_users` WRITE;
/*!40000 ALTER TABLE `tb_users` DISABLE KEYS */;
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
  `name` varchar(45) NOT NULL,
  `size` int(11) NOT NULL COMMENT 'volume 容量大小，单位byte',
  `VGname` varchar(45) NOT NULL,
  `driver` varchar(45) NOT NULL,
  `fstype` varchar(45) NOT NULL,
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

-- Dump completed on 2016-04-29 10:29:53
