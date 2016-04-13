-- MySQL dump 10.13  Distrib 5.6.24, for linux-glibc2.5 (x86_64)
--
-- Host: 192.168.16.11    Database: mg_dev
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
  `datacenter` varchar(64) DEFAULT NULL COMMENT '数据中心名称',
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
  `name` varchar(45) NOT NULL COMMENT '软件包名称',
  `docker_image_id` varchar(128) DEFAULT NULL COMMENT 'docker image id',
  `version` varchar(45) NOT NULL COMMENT '软件版本号',
  `config_file_path` varchar(1024) NOT NULL,
  `config_key_sets` varchar(4096) DEFAULT NULL COMMENT '配置文件中可修改key的集合\n[key1,key2]',
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
  `networking_id` varchar(128) NOT NULL COMMENT 'tb_network ID',
  `unit_id` varchar(128) DEFAULT NULL COMMENT '所属unit ID',
  `allocated` tinyint(1) DEFAULT '0' COMMENT '是否被分配，0为未分配，1为已分配',
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
  `type` tinyint(4) unsigned NOT NULL COMMENT '网络类型：管理网、内部业务网、外部接入网。',
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
  `admin_ip` int(11) unsigned NOT NULL COMMENT '内网网卡 IP，UINT32',
  `max_container` int(11) NOT NULL,
  `status` int(4) unsigned NOT NULL DEFAULT '0' COMMENT '物理机状态\n0	准备入库			import\n1	初始化安装中		installing\n2	初始化安装成功	installed\n3	初始化安装成功	installfailed\n4	测试中			testing\n5	测试失败			failedtest\n6	启用				enable\n7	停用				disable\n',
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
  `horus_distribution_ip` varchar(45) DEFAULT NULL COMMENT 'Horus distribution system HTTP IP',
  `horus_distribution_port` int(10) unsigned DEFAULT NULL COMMENT 'Horus distribution system HTTP Port',
  `horus_event_ip` varchar(45) DEFAULT NULL,
  `hours_event_port` int(10) unsigned DEFAULT NULL,
  `retry` tinyint(4) DEFAULT NULL COMMENT '失败重试次数',
  `registry_username` varchar(45) DEFAULT NULL,
  `registry_password` varchar(45) DEFAULT NULL,
  `registry_email` varchar(45) DEFAULT NULL,
  `registry_token` varchar(4096) DEFAULT NULL,
  `registry_domain` varchar(45) DEFAULT NULL,
  `registry_address` varchar(45) DEFAULT NULL,
  `registry_port` int(6) DEFAULT NULL,
  `registry_ca_crt` varchar(4096) DEFAULT NULL,
  `pkg_name` varchar(45) NOT NULL,
  `source_dir` varchar(128) NOT NULL,
  `destination_dir` varchar(128) NOT NULL,
  `script_name` varchar(45) NOT NULL,
  `ca_crt_name` varchar(45) NOT NULL,
  PRIMARY KEY (`id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8;
/*!40101 SET character_set_client = @saved_cs_client */;

--
-- Dumping data for table `tb_system_config`
--

LOCK TABLES `tb_system_config` WRITE;
/*!40000 ALTER TABLE `tb_system_config` DISABLE KEYS */;
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
  `name` varchar(45) NOT NULL COMMENT 'unit 名称，命名规则为<unit_id_8bit>_<server_name>',
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
  `id` int(11) NOT NULL,
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
  `size` int(11) NOT NULL COMMENT 'volume 容量大小，单位m',
  `VGname` varchar(45) NOT NULL,
  `driver` varchar(45) NOT NULL,
  `fstype` varchar(45) NOT NULL,
  PRIMARY KEY (`id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8;
/*!40101 SET character_set_client = @saved_cs_client */;

--
-- Dumping data for table `tb_volumes`
--

LOCK TABLES `tb_volumes` WRITE;
/*!40000 ALTER TABLE `tb_volumes` DISABLE KEYS */;
/*!40000 ALTER TABLE `tb_volumes` ENABLE KEYS */;
UNLOCK TABLES;

--
-- Table structure for table `tp_port`
--

DROP TABLE IF EXISTS `tp_port`;
/*!40101 SET @saved_cs_client     = @@character_set_client */;
/*!40101 SET character_set_client = utf8 */;
CREATE TABLE `tp_port` (
  `port` int(11) unsigned NOT NULL AUTO_INCREMENT COMMENT '端口号，10000+，分配给容器使用，容器的端口是唯一的。',
  `name` varchar(45) DEFAULT NULL,
  `unit_id` varchar(128) DEFAULT NULL,
  `proto` varchar(45) DEFAULT NULL,
  `allocated` tinyint(1) unsigned NOT NULL DEFAULT '0' COMMENT '是否被分配，0：未分配，1：已分配；',
  PRIMARY KEY (`port`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8;
/*!40101 SET character_set_client = @saved_cs_client */;

--
-- Dumping data for table `tp_port`
--

LOCK TABLES `tp_port` WRITE;
/*!40000 ALTER TABLE `tp_port` DISABLE KEYS */;
/*!40000 ALTER TABLE `tp_port` ENABLE KEYS */;
UNLOCK TABLES;
/*!40103 SET TIME_ZONE=@OLD_TIME_ZONE */;

/*!40101 SET SQL_MODE=@OLD_SQL_MODE */;
/*!40014 SET FOREIGN_KEY_CHECKS=@OLD_FOREIGN_KEY_CHECKS */;
/*!40014 SET UNIQUE_CHECKS=@OLD_UNIQUE_CHECKS */;
/*!40101 SET CHARACTER_SET_CLIENT=@OLD_CHARACTER_SET_CLIENT */;
/*!40101 SET CHARACTER_SET_RESULTS=@OLD_CHARACTER_SET_RESULTS */;
/*!40101 SET COLLATION_CONNECTION=@OLD_COLLATION_CONNECTION */;
/*!40111 SET SQL_NOTES=@OLD_SQL_NOTES */;

-- Dump completed on 2016-04-13 10:05:14
