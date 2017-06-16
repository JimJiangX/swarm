-- MySQL dump 10.13  Distrib 5.7.17, for macos10.12 (x86_64)
--
-- Host: 192.168.4.130    Database: mgm
-- ------------------------------------------------------
-- Server version	5.7.17-enterprise-commercial-advanced-log

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
-- Table structure for table `tbl_backup_files`
--

DROP TABLE IF EXISTS `tbl_backup_files`;
/*!40101 SET @saved_cs_client     = @@character_set_client */;
/*!40101 SET character_set_client = utf8 */;
CREATE TABLE `tbl_backup_files` (
  `ai` int(24) NOT NULL AUTO_INCREMENT COMMENT '自增字段,与业务无关',
  `id` varchar(128) NOT NULL,
  `strategy_id` varchar(128) NOT NULL COMMENT '关联的备份策略id',
  `task_id` varchar(128) NOT NULL COMMENT '关联tbl_dbaas_task.id',
  `unit_id` varchar(128) NOT NULL COMMENT '所属容器的id',
  `type` varchar(45) DEFAULT NULL COMMENT '全量／增量\n\nfull/incremental',
  `path` varchar(1024) DEFAULT NULL COMMENT '备份文件路径(包含文件名)',
  `size` bigint(128) unsigned DEFAULT NULL COMMENT '备份文件大小，单位：byte',
  `retention` datetime DEFAULT NULL COMMENT '到期日期',
  `created_at` datetime NOT NULL COMMENT '创建时间',
  `finished_at` datetime DEFAULT NULL COMMENT '完成时间',
  PRIMARY KEY (`ai`),
  UNIQUE KEY `id_UNIQUE` (`id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8;
/*!40101 SET character_set_client = @saved_cs_client */;

--
-- Dumping data for table `tbl_backup_files`
--

LOCK TABLES `tbl_backup_files` WRITE;
/*!40000 ALTER TABLE `tbl_backup_files` DISABLE KEYS */;
/*!40000 ALTER TABLE `tbl_backup_files` ENABLE KEYS */;
UNLOCK TABLES;

--
-- Table structure for table `tbl_backup_strategy`
--

DROP TABLE IF EXISTS `tbl_backup_strategy`;
/*!40101 SET @saved_cs_client     = @@character_set_client */;
/*!40101 SET character_set_client = utf8 */;
CREATE TABLE `tbl_backup_strategy` (
  `ai` int(24) NOT NULL AUTO_INCREMENT COMMENT '自增字段,与业务无关',
  `id` varchar(128) NOT NULL COMMENT '备份策略ID',
  `name` varchar(128) NOT NULL COMMENT '备份策略名称',
  `service_id` varchar(128) NOT NULL COMMENT '所属服务ID',
  `spec` varchar(45) NOT NULL COMMENT '触发规则描述，cron语法',
  `next` datetime DEFAULT NULL COMMENT '下次执行时间',
  `valid` datetime NOT NULL COMMENT '到期日期',
  `timeout` bigint(255) unsigned DEFAULT NULL COMMENT '执行备份的超时时长,time.Unix()值',
  `backup_dir` varchar(128) NOT NULL COMMENT '实例单元存放备份目录',
  `type` varchar(64) DEFAULT NULL COMMENT '备份类型\n全量／增量\nfull／incremental\n',
  `enabled` tinyint(1) unsigned DEFAULT '1' COMMENT '0:停用\n1:启用',
  `created_at` datetime DEFAULT NULL COMMENT '创建时间',
  PRIMARY KEY (`ai`),
  UNIQUE KEY `id_UNIQUE` (`id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8;
/*!40101 SET character_set_client = @saved_cs_client */;

--
-- Dumping data for table `tbl_backup_strategy`
--

LOCK TABLES `tbl_backup_strategy` WRITE;
/*!40000 ALTER TABLE `tbl_backup_strategy` DISABLE KEYS */;
/*!40000 ALTER TABLE `tbl_backup_strategy` ENABLE KEYS */;
UNLOCK TABLES;

--
-- Table structure for table `tbl_cluster`
--

DROP TABLE IF EXISTS `tbl_cluster`;
/*!40101 SET @saved_cs_client     = @@character_set_client */;
/*!40101 SET character_set_client = utf8 */;
CREATE TABLE `tbl_cluster` (
  `id` varchar(128) NOT NULL COMMENT '主键',
  `max_host` int(11) unsigned NOT NULL DEFAULT '500' COMMENT '最大物理机数量',
  `usage_limit` int(11) NOT NULL DEFAULT '80' COMMENT ' 集群中物理机资源使用上限比例百分比, 0-100',
  PRIMARY KEY (`id`),
  UNIQUE KEY `id_UNIQUE` (`id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8 COMMENT='集群表';
/*!40101 SET character_set_client = @saved_cs_client */;

--
-- Dumping data for table `tbl_cluster`
--

LOCK TABLES `tbl_cluster` WRITE;
/*!40000 ALTER TABLE `tbl_cluster` DISABLE KEYS */;
/*!40000 ALTER TABLE `tbl_cluster` ENABLE KEYS */;
UNLOCK TABLES;

--
-- Table structure for table `tbl_host`
--

DROP TABLE IF EXISTS `tbl_host`;
/*!40101 SET @saved_cs_client     = @@character_set_client */;
/*!40101 SET character_set_client = utf8 */;
CREATE TABLE `tbl_host` (
  `id` varchar(128) NOT NULL COMMENT '主键',
  `admin_ip` varchar(64) NOT NULL COMMENT '管理IP地址\n例如192.168.2.100',
  `cluster_id` varchar(128) NOT NULL COMMENT '所属集群ID',
  `engine_id` varchar(128) DEFAULT NULL COMMENT 'docker engine id',
  `room` varchar(128) NOT NULL COMMENT '机房编号',
  `seat` varchar(128) NOT NULL COMMENT '机架编号',
  `storage` varchar(128) DEFAULT NULL COMMENT '存储系统ID',
  `max_container` int(11) NOT NULL COMMENT '可容纳最大容器数量',
  `nfs_ip` varchar(45) NOT NULL COMMENT 'nfs IP地址',
  `nfs_dir` varchar(128) NOT NULL COMMENT 'nfs 源目录',
  `nfs_mount_dir` varchar(128) NOT NULL COMMENT 'nfs 挂载目录',
  `nfs_mount_opts` varchar(256) DEFAULT NULL COMMENT 'nfs 挂载参数',
  `status` tinyint(4) unsigned DEFAULT NULL COMMENT '状态\n目前不传送给前端，原因是前端通过主机入库时的任务状态描述入库状态',
  `enabled` tinyint(1) unsigned NOT NULL DEFAULT '1' COMMENT '是否可用\n0	fasle\n1	true',
  `register_at` datetime DEFAULT NULL COMMENT '注册入库完成时间',
  PRIMARY KEY (`id`,`admin_ip`),
  UNIQUE KEY `id_UNIQUE` (`id`),
  UNIQUE KEY `admin_ip_UNIQUE` (`admin_ip`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8 COMMENT='物理机表，\n主机固有属性存放在dockerd中';
/*!40101 SET character_set_client = @saved_cs_client */;

--
-- Dumping data for table `tbl_host`
--

LOCK TABLES `tbl_host` WRITE;
/*!40000 ALTER TABLE `tbl_host` DISABLE KEYS */;
/*!40000 ALTER TABLE `tbl_host` ENABLE KEYS */;
UNLOCK TABLES;

--
-- Table structure for table `tbl_ip`
--

DROP TABLE IF EXISTS `tbl_ip`;
/*!40101 SET @saved_cs_client     = @@character_set_client */;
/*!40101 SET character_set_client = utf8 */;
CREATE TABLE `tbl_ip` (
  `ip_addr` int(11) unsigned NOT NULL COMMENT 'IP地址\nencoding into uint32 by Big-Endian',
  `prefix` int(11) NOT NULL COMMENT 'IP 掩码\n值范围0～32',
  `networking_id` varchar(128) NOT NULL COMMENT '所属网段ID，网段ID为前端对象ID，作为过滤条件使用，mgm没有网段对象',
  `gateway` varchar(64) NOT NULL COMMENT '网关IP地址',
  `vlan_id` int(11) NOT NULL COMMENT 'VLAN ID',
  `engine_id` varchar(128) DEFAULT NULL COMMENT '所属主机docker engine ID',
  `net_dev` varchar(45) DEFAULT NULL COMMENT '使用网卡设备名称',
  `unit_id` varchar(128) DEFAULT NULL COMMENT '所属单元ID',
  `enabled` tinyint(1) NOT NULL DEFAULT '1' COMMENT '是否被可用\n0	fasle\n1	true',
  `bandwidth` int(11) DEFAULT NULL,
  PRIMARY KEY (`ip_addr`),
  UNIQUE KEY `ip_addr_UNIQUE` (`ip_addr`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8 COMMENT='IP地址表';
/*!40101 SET character_set_client = @saved_cs_client */;

--
-- Dumping data for table `tbl_ip`
--

LOCK TABLES `tbl_ip` WRITE;
/*!40000 ALTER TABLE `tbl_ip` DISABLE KEYS */;
/*!40000 ALTER TABLE `tbl_ip` ENABLE KEYS */;
UNLOCK TABLES;

--
-- Table structure for table `tbl_san`
--

DROP TABLE IF EXISTS `tbl_san`;
/*!40101 SET @saved_cs_client     = @@character_set_client */;
/*!40101 SET character_set_client = utf8 */;
CREATE TABLE `tbl_san` (
  `ai` int(24) NOT NULL AUTO_INCREMENT COMMENT '自增字段,与业务无关',
  `id` varchar(128) NOT NULL COMMENT 'storage_system_ID',
  `vendor` varchar(128) NOT NULL COMMENT '厂商:HUAWEI / HITACHI',
  `admin_unit` varchar(128) NOT NULL COMMENT '管理域名称,HDS专有',
  `lun_start` int(11) unsigned NOT NULL COMMENT '起始位,HDS专有',
  `lun_end` int(11) unsigned NOT NULL COMMENT '结束位,HDS专有',
  `hlu_start` int(11) unsigned NOT NULL COMMENT 'host_lun_start ',
  `hlu_end` int(11) unsigned NOT NULL COMMENT 'host_un_end',
  PRIMARY KEY (`ai`),
  UNIQUE KEY `id_UNIQUE` (`id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8;
/*!40101 SET character_set_client = @saved_cs_client */;

--
-- Dumping data for table `tbl_san`
--

LOCK TABLES `tbl_san` WRITE;
/*!40000 ALTER TABLE `tbl_san` DISABLE KEYS */;
/*!40000 ALTER TABLE `tbl_san` ENABLE KEYS */;
UNLOCK TABLES;

--
-- Table structure for table `tbl_san_raid_group`
--

DROP TABLE IF EXISTS `tbl_san_raid_group`;
/*!40101 SET @saved_cs_client     = @@character_set_client */;
/*!40101 SET character_set_client = utf8 */;
CREATE TABLE `tbl_san_raid_group` (
  `ai` int(24) NOT NULL AUTO_INCREMENT COMMENT '自增字段,与业务无关',
  `id` varchar(128) NOT NULL COMMENT '存储RAID ID',
  `storage_rg_id` varchar(11) NOT NULL COMMENT '在存储系统上的Raid group ID',
  `storage_system_id` varchar(128) NOT NULL COMMENT 'RAID GROUP 所属存储系统ID',
  `enabled` tinyint(1) unsigned NOT NULL DEFAULT '1' COMMENT '是否启用\n0:停用\n1:启用',
  PRIMARY KEY (`ai`),
  UNIQUE KEY `id_UNIQUE` (`storage_system_id`,`storage_rg_id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8;
/*!40101 SET character_set_client = @saved_cs_client */;

--
-- Dumping data for table `tbl_san_raid_group`
--

LOCK TABLES `tbl_san_raid_group` WRITE;
/*!40000 ALTER TABLE `tbl_san_raid_group` DISABLE KEYS */;
/*!40000 ALTER TABLE `tbl_san_raid_group` ENABLE KEYS */;
UNLOCK TABLES;

--
-- Table structure for table `tbl_san_raid_group_lun`
--

DROP TABLE IF EXISTS `tbl_san_raid_group_lun`;
/*!40101 SET @saved_cs_client     = @@character_set_client */;
/*!40101 SET character_set_client = utf8 */;
CREATE TABLE `tbl_san_raid_group_lun` (
  `ai` int(24) NOT NULL AUTO_INCREMENT COMMENT '自增字段,与业务无关',
  `id` varchar(128) NOT NULL COMMENT 'LUN ID',
  `san_lun_id` int(11) NOT NULL COMMENT '在存储系统上的LUN ID',
  `name` varchar(128) NOT NULL COMMENT 'LUN 名称',
  `raid_group_id` varchar(128) NOT NULL COMMENT '所属Raid Group ID(平台分配的ID)',
  `size` bigint(128) NOT NULL COMMENT 'LUN 容量大小,单位为byte',
  `vg_name` varchar(128) DEFAULT NULL COMMENT '所属Volume Group 名称',
  `mapping_hostname` varchar(128) DEFAULT NULL COMMENT 'LUN 映射主机名称',
  `host_lun_id` int(11) DEFAULT NULL COMMENT '在映射主机上的LUN ID',
  `created_at` datetime NOT NULL COMMENT '创建日期',
  PRIMARY KEY (`ai`),
  UNIQUE KEY `id_UNIQUE` (`id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8;
/*!40101 SET character_set_client = @saved_cs_client */;

--
-- Dumping data for table `tbl_san_raid_group_lun`
--

LOCK TABLES `tbl_san_raid_group_lun` WRITE;
/*!40000 ALTER TABLE `tbl_san_raid_group_lun` DISABLE KEYS */;
/*!40000 ALTER TABLE `tbl_san_raid_group_lun` ENABLE KEYS */;
UNLOCK TABLES;

--
-- Table structure for table `tbl_service`
--

DROP TABLE IF EXISTS `tbl_service`;
/*!40101 SET @saved_cs_client     = @@character_set_client */;
/*!40101 SET character_set_client = utf8 */;
CREATE TABLE `tbl_service` (
  `id` varchar(128) NOT NULL COMMENT '主键',
  `name` varchar(128) NOT NULL COMMENT '服务名称',
  `description_id` varchar(128) NOT NULL COMMENT '服务描述信息',
  `tag` varchar(128) DEFAULT NULL,
  `high_available` tinyint(1) NOT NULL,
  `auto_healing` tinyint(1) unsigned NOT NULL DEFAULT '0' COMMENT '服务自动愈合\n0:停用\n1:启用',
  `auto_scaling` tinyint(1) unsigned NOT NULL DEFAULT '0' COMMENT '自动扩缩\n0	停用\n1	启用',
  `backup_max_size` bigint(128) unsigned DEFAULT NULL COMMENT '备份文件总大小,单位:byte',
  `backup_files_retention` bigint(128) DEFAULT NULL COMMENT '文件保存时间,单位:Hour',
  `action_status` int(10) unsigned DEFAULT NULL COMMENT '操作动作状态\n',
  `created_at` datetime NOT NULL COMMENT '创建日期',
  `finished_at` datetime DEFAULT NULL COMMENT '创建完成日期',
  PRIMARY KEY (`id`),
  UNIQUE KEY `id_UNIQUE` (`id`),
  UNIQUE KEY `name_UNIQUE` (`name`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8;
/*!40101 SET character_set_client = @saved_cs_client */;

--
-- Dumping data for table `tbl_service`
--

LOCK TABLES `tbl_service` WRITE;
/*!40000 ALTER TABLE `tbl_service` DISABLE KEYS */;
/*!40000 ALTER TABLE `tbl_service` ENABLE KEYS */;
UNLOCK TABLES;

--
-- Table structure for table `tbl_service_decription`
--

DROP TABLE IF EXISTS `tbl_service_decription`;
/*!40101 SET @saved_cs_client     = @@character_set_client */;
/*!40101 SET character_set_client = utf8 */;
CREATE TABLE `tbl_service_decription` (
  `id` varchar(128) NOT NULL,
  `service_id` varchar(128) NOT NULL,
  `architecture` varchar(128) NOT NULL COMMENT '服务结构描述\n例如：m:1#sb:1#sl:1\n	  m:3',
  `schedule_opts` varchar(512) NOT NULL,
  `unit_num` int(11) NOT NULL,
  `cpu_num` int(11) NOT NULL,
  `mem_size` bigint(20) NOT NULL,
  `image_id` varchar(128) NOT NULL,
  `image_version` varchar(128) NOT NULL,
  `volumes` longtext NOT NULL,
  `networks` longtext NOT NULL,
  `cluster_id` varchar(128) NOT NULL,
  `options` varchar(128) DEFAULT NULL,
  `previous_version` varchar(128) DEFAULT NULL,
  PRIMARY KEY (`id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8;
/*!40101 SET character_set_client = @saved_cs_client */;

--
-- Dumping data for table `tbl_service_decription`
--

LOCK TABLES `tbl_service_decription` WRITE;
/*!40000 ALTER TABLE `tbl_service_decription` DISABLE KEYS */;
/*!40000 ALTER TABLE `tbl_service_decription` ENABLE KEYS */;
UNLOCK TABLES;

--
-- Table structure for table `tbl_software_image`
--

DROP TABLE IF EXISTS `tbl_software_image`;
/*!40101 SET @saved_cs_client     = @@character_set_client */;
/*!40101 SET character_set_client = utf8 */;
CREATE TABLE `tbl_software_image` (
  `id` varchar(128) NOT NULL COMMENT '主键',
  `software_name` varchar(64) NOT NULL COMMENT '所属软件名称',
  `major_version` int(11) NOT NULL COMMENT '主版本号',
  `minor_version` int(11) NOT NULL COMMENT '次版本号',
  `patch_version` int(11) NOT NULL COMMENT '修订版本号',
  `docker_image_id` varchar(128) DEFAULT NULL COMMENT 'docker image id',
  `size` int(11) DEFAULT NULL,
  `label` varchar(4096) DEFAULT NULL COMMENT '预留备注',
  `upload_at` datetime NOT NULL COMMENT '上传日期',
  PRIMARY KEY (`id`),
  UNIQUE KEY `id_UNIQUE` (`id`),
  UNIQUE KEY `index3` (`software_name`,`major_version`,`minor_version`,`patch_version`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8 COMMENT='软件镜像表';
/*!40101 SET character_set_client = @saved_cs_client */;

--
-- Dumping data for table `tbl_software_image`
--

LOCK TABLES `tbl_software_image` WRITE;
/*!40000 ALTER TABLE `tbl_software_image` DISABLE KEYS */;
/*!40000 ALTER TABLE `tbl_software_image` ENABLE KEYS */;
UNLOCK TABLES;

--
-- Table structure for table `tbl_system_config`
--

DROP TABLE IF EXISTS `tbl_system_config`;
/*!40101 SET @saved_cs_client     = @@character_set_client */;
/*!40101 SET character_set_client = utf8 */;
CREATE TABLE `tbl_system_config` (
  `dc_id` int(11) NOT NULL COMMENT '站点ID',
  `retry` tinyint(4) DEFAULT NULL COMMENT '资源分配失败重试次数',
  `consul_ip` varchar(128) NOT NULL COMMENT 'Consul server IP地址, 包含多个IP',
  `consul_port` int(10) unsigned NOT NULL COMMENT 'Consul Server HTTP Port',
  `consul_dc` varchar(45) NOT NULL COMMENT 'Consul DataCenter',
  `consul_token` varchar(45) DEFAULT NULL COMMENT 'Consul Token,可以为空',
  `consul_wait_time` int(10) DEFAULT NULL,
  `docker_port` int(10) unsigned NOT NULL COMMENT 'docker Port',
  `plugin_port` int(6) NOT NULL COMMENT 'docker volume plugin port',
  `swarm_agent_port` int(11) NOT NULL,
  `registry_os_username` varchar(45) NOT NULL COMMENT 'registry 操作系统用户',
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
  `backup_dir` varchar(128) NOT NULL COMMENT '挂载到容器内的备份目录',
  PRIMARY KEY (`dc_id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8;
/*!40101 SET character_set_client = @saved_cs_client */;

--
-- Dumping data for table `tbl_system_config`
--

LOCK TABLES `tbl_system_config` WRITE;
/*!40000 ALTER TABLE `tbl_system_config` DISABLE KEYS */;
INSERT INTO `tbl_system_config` VALUES (1,0,'<CONSUL_SERVER1>,<CONSUL_SERVER2>,<CONSUL_SERVER3>',<CONSUL_PORT>,'dc1',NULL,0,2375,<MGM_PLUGIN_PORT>,4123,'root','<DOCKER_REG_DOMAIN>','<DOCKER_REG_IP>',<REG_PORT>,'<DOCKER_REG_USER>','<DOCKER_REG_PASSWORD>','@bsgchina.com','-----BEGIN CERTIFICATE-----\nMIIFNjCCAx4CCQDfgDH4S2oKJzANBgkqhkiG9w0BAQUFADBdMQswCQYDVQQGEwJD\nTjERMA8GA1UECAwIc2hhbmdoYWkxDzANBgNVBAcMBnB1ZG9uZzEMMAoGA1UECgwD\nQlNHMRwwGgYDVQQDDBNyZWdpc3RyeS5kYnNjYWxlLm1lMB4XDTE3MDYwODAxNDAz\nNVoXDTI3MDYwNjAxNDAzNVowXTELMAkGA1UEBhMCQ04xETAPBgNVBAgMCHNoYW5n\naGFpMQ8wDQYDVQQHDAZwdWRvbmcxDDAKBgNVBAoMA0JTRzEcMBoGA1UEAwwTcmVn\naXN0cnkuZGJzY2FsZS5tZTCCAiIwDQYJKoZIhvcNAQEBBQADggIPADCCAgoCggIB\nANgMAVIeD/+QjIzSE0APrlzbTsn7K51fBoix0rgkzTNi26PYhrRumQDv5gKhtKgM\nxp9EAIT+tj2JKjgfe40MQIpjy4DNqwAC0KGanY0gYa9WudeNleGuGfke4QZ6shhS\nkHQYCcDxudXB1zge+dQQVTm6KTphxAIxKJucmKZygTpWDHzK2PuwrPFMc00DiyeQ\nv/h3fuJN60+HfRoWOCfw0BdiIuL5pVjYxc0wYpBVo8r6FIstC3mhowpneBV5L/C+\nlUZ2cRbE+qh4IjQDNeabCESuTsDi/SW+KdOJ13A/usurNJmhyH3rDQNllGRjygJH\neyL5WDGNeoLkOt3WYOKbIijGvBVdXd2VKt0z1rL7CMjEFe8S4yzyoPKwr4CI+YYw\nZcWQSgAO5QxlLU8pksOI65StEXF82d93UjeHaA1eMhwxHIG+wh7ZTMDeNWIUnF9g\n9fp2sEIQX1MXtZiH1Pp9H6tCqESZCajT3XXKbFVdIOjA5nfDp8lLr6PFEhR3k0wu\nbz1CFGxFrxLpctbi3IqdjyA/xskBmrJ/NYcMipskiTwzefmWjPpSYwHOfP8uqxCu\nB7N+5+2f1XUIUhI33B5dGLvc3lXnxJCSS2PqOxTKc0LP3eb1BnxSlYdIA4WeyNqi\n2WDH6Lt+hS04WBmmyGY0tUJBRi1R3us077cwKD5wKj4/AgMBAAEwDQYJKoZIhvcN\nAQEFBQADggIBAHXjIn4itrS9A2OkccyN0VSsUdyHxOmnEozZX0/hYbAf2HfcrEc4\nLcwnckEc+RbPQvDKR+Bnilkm+2/hpXottMHtrhyWrCz8p3Lr/S6jvh++qgL17kIs\nGJYuvkjTK0dxQZMvRpgQ2/SkIrfXmgVIFQwVSzqJZ4x0NNQ6GcBcjqwcineRyinX\n4lIqWMirXr26tKcDIRSlt36DXoVCMkpXNTVonwmOymB1Y8izn4XCnh8+Z6I4t9QY\nIOcQVhgx9LAO+eP0qUndUAvZIw3YSRhS11iX10RVqg6ShtRPApHBlc3TVu/l9kOM\nF3TEawMRSAeuLMHaxQ4bbhjt2DRNyQkLy8bsGsxBXhLxUM+TkV2uYlMMaJoHIw+7\n6OOGHwmxVJ8YLqxHtg3rbuw7h0dCwZhtS3noAyK+FWBvjqUrMOUmEn6D7VHHhBev\nxUUfy9asV/8Bh7mP87G36gmtAceTSvoZLaZ/lHcrmSyyYUuUuG0FpOG67INRUHWY\nHqQvpaWyhvh6XTOvBNwnkSHdBR4z3aICsSwxvmF+gC7yXhOCvUUAmC+q5hFG5I2b\ntiZb5cic6I7gRqAbiZouZ6o6Otj7D5XALb/L8zisj7hgQtl43pmnKCoPMOLbwVYm\neKwB7j7UEFZijv1i2g4pRq37LsnfSaGW909nJEuP26FrodAkclVZXjuD\n-----END CERTIFICATE-----',NULL,'./script/node-init','/tmp','node-init.sh','node-clean.sh','registery-ca.crt','/BACKUP');
/*!40000 ALTER TABLE `tbl_system_config` ENABLE KEYS */;
UNLOCK TABLES;

--
-- Table structure for table `tbl_task`
--

DROP TABLE IF EXISTS `tbl_task`;
/*!40101 SET @saved_cs_client     = @@character_set_client */;
/*!40101 SET character_set_client = utf8 */;
CREATE TABLE `tbl_task` (
  `id` varchar(128) NOT NULL COMMENT '主键\n任务ID',
  `name` varchar(128) NOT NULL,
  `related` varchar(128) NOT NULL COMMENT '关联表名称或者对象',
  `link_to` varchar(128) NOT NULL COMMENT '关联ID',
  `link_table` varchar(45) DEFAULT NULL,
  `description` varchar(128) DEFAULT NULL COMMENT '描述',
  `labels` varchar(512) DEFAULT NULL,
  `errors` longtext,
  `timeout` bigint(128) unsigned DEFAULT NULL,
  `status` tinyint(4) unsigned DEFAULT '0' COMMENT '任务状态\n1	创建任务				create\n2	执行中				running		\n3	任务中止,未完成		stop\n4	任务未执行，被取消		cancel\n5	任务完成				done\n6	任务超时				timeout\n7	任务失败				faile',
  `created_at` datetime NOT NULL COMMENT '创建时间',
  `finished_at` datetime DEFAULT NULL COMMENT '完成时间',
  `timestamp` bigint(128) DEFAULT NULL COMMENT '时间戳',
  PRIMARY KEY (`id`),
  UNIQUE KEY `id_UNIQUE` (`id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8;
/*!40101 SET character_set_client = @saved_cs_client */;

--
-- Dumping data for table `tbl_task`
--

LOCK TABLES `tbl_task` WRITE;
/*!40000 ALTER TABLE `tbl_task` DISABLE KEYS */;
/*!40000 ALTER TABLE `tbl_task` ENABLE KEYS */;
UNLOCK TABLES;

--
-- Table structure for table `tbl_unit`
--

DROP TABLE IF EXISTS `tbl_unit`;
/*!40101 SET @saved_cs_client     = @@character_set_client */;
/*!40101 SET character_set_client = utf8 */;
CREATE TABLE `tbl_unit` (
  `id` varchar(128) NOT NULL COMMENT 'unit ID',
  `service_id` varchar(128) NOT NULL COMMENT ' 所属 Service ID',
  `name` varchar(128) NOT NULL COMMENT 'unit 名称,命名规则为<unit_id_8bit>_<service_name>',
  `type` varchar(45) NOT NULL COMMENT '单元的软件类型名称\nswitch_manager \nupproxy \nupsql\nmysql\nredis',
  `network_mode` varchar(45) NOT NULL DEFAULT 'none' COMMENT '网络模式\n默认 none\n',
  `engine_id` varchar(128) DEFAULT NULL COMMENT '所在docker engine ID',
  `container_id` varchar(128) DEFAULT NULL COMMENT 'docker 生成的ID',
  `status` int(11) unsigned NOT NULL COMMENT '状态',
  `created_at` datetime NOT NULL,
  `latest_error` longtext COMMENT '最新错误信息',
  PRIMARY KEY (`id`),
  UNIQUE KEY `id_UNIQUE` (`id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8;
/*!40101 SET character_set_client = @saved_cs_client */;

--
-- Dumping data for table `tbl_unit`
--

LOCK TABLES `tbl_unit` WRITE;
/*!40000 ALTER TABLE `tbl_unit` DISABLE KEYS */;
/*!40000 ALTER TABLE `tbl_unit` ENABLE KEYS */;
UNLOCK TABLES;

--
-- Table structure for table `tbl_volume`
--

DROP TABLE IF EXISTS `tbl_volume`;
/*!40101 SET @saved_cs_client     = @@character_set_client */;
/*!40101 SET character_set_client = utf8 */;
CREATE TABLE `tbl_volume` (
  `id` varchar(128) NOT NULL COMMENT 'volume ID',
  `unit_id` varchar(128) NOT NULL COMMENT '所属Unit ID',
  `engine_id` varchar(128) DEFAULT NULL,
  `name` varchar(128) NOT NULL COMMENT '名称',
  `size` bigint(128) NOT NULL COMMENT 'volume 容量大小,单位:byte',
  `vg` varchar(128) NOT NULL COMMENT '所属Volume Group',
  `driver_type` varchar(45) NOT NULL,
  `driver` varchar(45) NOT NULL COMMENT 'docker plugin 驱动名称',
  `fstype` varchar(45) NOT NULL COMMENT '文件系统类型',
  PRIMARY KEY (`id`),
  UNIQUE KEY `id_UNIQUE` (`id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8;
/*!40101 SET character_set_client = @saved_cs_client */;

--
-- Dumping data for table `tbl_volume`
--

LOCK TABLES `tbl_volume` WRITE;
/*!40000 ALTER TABLE `tbl_volume` DISABLE KEYS */;
/*!40000 ALTER TABLE `tbl_volume` ENABLE KEYS */;
UNLOCK TABLES;
SET @@SESSION.SQL_LOG_BIN = @MYSQLDUMP_TEMP_LOG_BIN;
/*!40103 SET TIME_ZONE=@OLD_TIME_ZONE */;

/*!40101 SET SQL_MODE=@OLD_SQL_MODE */;
/*!40014 SET FOREIGN_KEY_CHECKS=@OLD_FOREIGN_KEY_CHECKS */;
/*!40014 SET UNIQUE_CHECKS=@OLD_UNIQUE_CHECKS */;
/*!40101 SET CHARACTER_SET_CLIENT=@OLD_CHARACTER_SET_CLIENT */;
/*!40101 SET CHARACTER_SET_RESULTS=@OLD_CHARACTER_SET_RESULTS */;
/*!40101 SET COLLATION_CONNECTION=@OLD_COLLATION_CONNECTION */;
/*!40111 SET SQL_NOTES=@OLD_SQL_NOTES */;

-- Dump completed on 2017-06-12 11:09:48
