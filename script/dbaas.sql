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
-- Table structure for table `tbl_dbaas_backup_files`
--

DROP TABLE IF EXISTS `tbl_dbaas_backup_files`;
/*!40101 SET @saved_cs_client     = @@character_set_client */;
/*!40101 SET character_set_client = utf8 */;
CREATE TABLE `tbl_dbaas_backup_files` (
  `ai` int(24) NOT NULL AUTO_INCREMENT COMMENT '自增字段，与业务无关',
  `id` varchar(128) NOT NULL,
  `strategy_id` varchar(128) NOT NULL COMMENT '关联的备份策略id',
  `task_id` varchar(128) NOT NULL COMMENT '关联tbl_dbaas_task.id',
  `unit_id` varchar(128) NOT NULL COMMENT '所属容器的id',
  `type` varchar(45) DEFAULT NULL COMMENT '全量／增量\n\nfull/incremental',
  `path` varchar(1024) DEFAULT NULL COMMENT '备份文件路径（包含文件名�?,
  `size` bigint(128) unsigned DEFAULT NULL COMMENT '备份文件大小，单位：byte',
  `retention` datetime DEFAULT NULL COMMENT '到期日期',
  `created_at` datetime NOT NULL COMMENT '创建时间',
  `finished_at` datetime DEFAULT NULL COMMENT '完成时间',
  PRIMARY KEY (`ai`),
  UNIQUE KEY `id_UNIQUE` (`id`)
) ENGINE=InnoDB AUTO_INCREMENT=38 DEFAULT CHARSET=utf8;
/*!40101 SET character_set_client = @saved_cs_client */;

--
-- Table structure for table `tbl_dbaas_backup_strategy`
--

DROP TABLE IF EXISTS `tbl_dbaas_backup_strategy`;
/*!40101 SET @saved_cs_client     = @@character_set_client */;
/*!40101 SET character_set_client = utf8 */;
CREATE TABLE `tbl_dbaas_backup_strategy` (
  `ai` int(24) NOT NULL AUTO_INCREMENT COMMENT '自增字段，与业务无关',
  `id` varchar(128) NOT NULL COMMENT '备份策略ID',
  `name` varchar(128) NOT NULL COMMENT '备份策略名称',
  `service_id` varchar(128) NOT NULL COMMENT '所属服务ID',
  `spec` varchar(45) NOT NULL COMMENT '触发规则描述，cron语法',
  `next` datetime DEFAULT NULL COMMENT '下一次执行时�?,
  `valid` datetime NOT NULL COMMENT '到期日期',
  `timeout` bigint(255) unsigned DEFAULT NULL COMMENT '执行备份的超时时长，time.Unix()值�?,
  `backup_dir` varchar(128) NOT NULL COMMENT '实例单元存放备份目录',
  `type` varchar(64) DEFAULT NULL COMMENT '备份类型\n全量／增量\nfull／incremental\n',
  `enabled` tinyint(1) unsigned DEFAULT '1' COMMENT '0:停用�?：启�?,
  `created_at` datetime DEFAULT NULL COMMENT '创建时间',
  PRIMARY KEY (`ai`),
  UNIQUE KEY `id_UNIQUE` (`id`)
) ENGINE=InnoDB AUTO_INCREMENT=38 DEFAULT CHARSET=utf8;
/*!40101 SET character_set_client = @saved_cs_client */;

--
-- Table structure for table `tbl_dbaas_cluster`
--

DROP TABLE IF EXISTS `tbl_dbaas_cluster`;
/*!40101 SET @saved_cs_client     = @@character_set_client */;
/*!40101 SET character_set_client = utf8 */;
CREATE TABLE `tbl_dbaas_cluster` (
  `ai` int(24) NOT NULL AUTO_INCREMENT COMMENT '自增字段，与业务无关',
  `id` varchar(128) NOT NULL COMMENT '主键',
  `name` varchar(128) NOT NULL COMMENT '集群名称',
  `type` varchar(64) NOT NULL COMMENT '集群类型\nupsql / upproxy',
  `storage_type` varchar(64) NOT NULL COMMENT '存储类型\nlocal, san',
  `storage_id` varchar(128) DEFAULT NULL COMMENT '关联存储系统ID',
  `networking_id` varchar(128) DEFAULT NULL COMMENT '关联的网段ID，当类型是proxy 时，需要关联网�?,
  `max_node` int(11) unsigned NOT NULL DEFAULT '500' COMMENT '最大物理机数量',
  `usage_limit` float NOT NULL DEFAULT '0.8' COMMENT '物理机资源使用上限比率�?-1',
  `enabled` tinyint(1) unsigned NOT NULL DEFAULT '1' COMMENT '集群状态，0：停用，1：启用�?,
  PRIMARY KEY (`ai`),
  UNIQUE KEY `id_UNIQUE` (`id`),
  UNIQUE KEY `name_UNIQUE` (`name`)
) ENGINE=InnoDB AUTO_INCREMENT=10 DEFAULT CHARSET=utf8;
/*!40101 SET character_set_client = @saved_cs_client */;

--
-- Table structure for table `tbl_dbaas_image`
--

DROP TABLE IF EXISTS `tbl_dbaas_image`;
/*!40101 SET @saved_cs_client     = @@character_set_client */;
/*!40101 SET character_set_client = utf8 */;
CREATE TABLE `tbl_dbaas_image` (
  `ai` int(24) NOT NULL AUTO_INCREMENT COMMENT '自增字段，与业务无关',
  `id` varchar(128) NOT NULL COMMENT '主键',
  `name` varchar(128) NOT NULL COMMENT '软件包名�?,
  `size` bigint(128) DEFAULT NULL COMMENT '镜像大小，单位byte',
  `docker_image_id` varchar(128) DEFAULT NULL COMMENT 'docker image id',
  `version` varchar(128) NOT NULL COMMENT '软件版本�?,
  `template_config_id` varchar(128) DEFAULT NULL COMMENT '配置文件模版ID',
  `label` varchar(4096) DEFAULT NULL COMMENT '备注',
  `config_keysets` longtext COMMENT '配置文件中，键值对的描述\n',
  `enabled` tinyint(1) unsigned DEFAULT '1' COMMENT '可用状态，0：停用，1：启用�?,
  `upload_at` datetime NOT NULL COMMENT '上传日期',
  PRIMARY KEY (`ai`),
  UNIQUE KEY `id_UNIQUE` (`id`)
) ENGINE=InnoDB AUTO_INCREMENT=36 DEFAULT CHARSET=utf8;
/*!40101 SET character_set_client = @saved_cs_client */;

--
-- Table structure for table `tbl_dbaas_ip`
--

DROP TABLE IF EXISTS `tbl_dbaas_ip`;
/*!40101 SET @saved_cs_client     = @@character_set_client */;
/*!40101 SET character_set_client = utf8 */;
CREATE TABLE `tbl_dbaas_ip` (
  `ai` int(24) NOT NULL AUTO_INCREMENT COMMENT '自增字段，与业务无关',
  `ip_addr` int(11) unsigned NOT NULL COMMENT 'IP地址,encoding into uint32 by Big-Endian',
  `prefix` int(11) unsigned NOT NULL COMMENT 'IP 掩码�?�?2',
  `networking_id` varchar(128) NOT NULL COMMENT '所属networking ID',
  `unit_id` varchar(128) DEFAULT NULL COMMENT '所属unit ID',
  `allocated` tinyint(1) NOT NULL DEFAULT '0' COMMENT '是否被分配，0为未分配�?为已分配',
  PRIMARY KEY (`ai`),
  UNIQUE KEY `ip_addr_UNIQUE` (`ip_addr`)
) ENGINE=InnoDB AUTO_INCREMENT=543 DEFAULT CHARSET=utf8;
/*!40101 SET character_set_client = @saved_cs_client */;

--
-- Table structure for table `tbl_dbaas_lun`
--

DROP TABLE IF EXISTS `tbl_dbaas_lun`;
/*!40101 SET @saved_cs_client     = @@character_set_client */;
/*!40101 SET character_set_client = utf8 */;
CREATE TABLE `tbl_dbaas_lun` (
  `ai` int(24) NOT NULL AUTO_INCREMENT COMMENT '自增字段，与业务无关',
  `id` varchar(128) NOT NULL COMMENT 'LUN ID',
  `storage_lun_id` int(11) NOT NULL COMMENT '在存储系统上的LUN ID',
  `name` varchar(128) NOT NULL COMMENT 'LUN 名称',
  `storage_system_id` varchar(128) NOT NULL COMMENT '所属存储系统ID',
  `raid_group_id` varchar(128) NOT NULL COMMENT '所属Raid Group ID(平台分配的ID)',
  `size` bigint(128) NOT NULL COMMENT 'LUN 容量大小，单位为byte',
  `vg_name` varchar(128) DEFAULT NULL COMMENT '所属Volume Group 名称',
  `mapping_hostname` varchar(128) DEFAULT NULL COMMENT 'LUN 映射主机名称',
  `host_lun_id` int(11) DEFAULT NULL COMMENT '在映射主机上的LUN ID',
  `created_at` datetime NOT NULL COMMENT '创建日期',
  PRIMARY KEY (`ai`),
  UNIQUE KEY `id_UNIQUE` (`id`)
) ENGINE=InnoDB AUTO_INCREMENT=27 DEFAULT CHARSET=utf8;
/*!40101 SET character_set_client = @saved_cs_client */;

--
-- Table structure for table `tbl_dbaas_networking`
--

DROP TABLE IF EXISTS `tbl_dbaas_networking`;
/*!40101 SET @saved_cs_client     = @@character_set_client */;
/*!40101 SET character_set_client = utf8 */;
CREATE TABLE `tbl_dbaas_networking` (
  `ai` int(24) NOT NULL AUTO_INCREMENT COMMENT '自增字段，与业务无关',
  `id` varchar(128) NOT NULL COMMENT '网段ID',
  `type` varchar(64) NOT NULL COMMENT '网络类型：\ninternal_access_networking	内部业务�?\nexternal_access_networking	外部接入�?,
  `gateway` varchar(64) NOT NULL COMMENT '网关IP',
  `enabled` tinyint(1) unsigned DEFAULT '1' COMMENT '0：停用，1：启�?,
  PRIMARY KEY (`ai`),
  UNIQUE KEY `id_UNIQUE` (`id`)
) ENGINE=InnoDB AUTO_INCREMENT=5 DEFAULT CHARSET=utf8;
/*!40101 SET character_set_client = @saved_cs_client */;

--
-- Table structure for table `tbl_dbaas_node`
--

DROP TABLE IF EXISTS `tbl_dbaas_node`;
/*!40101 SET @saved_cs_client     = @@character_set_client */;
/*!40101 SET character_set_client = utf8 */;
CREATE TABLE `tbl_dbaas_node` (
  `ai` int(24) NOT NULL AUTO_INCREMENT COMMENT '自增字段，与业务无关',
  `id` varchar(128) NOT NULL COMMENT 'node ID',
  `name` varchar(128) NOT NULL COMMENT 'node 名称',
  `cluster_id` varchar(128) NOT NULL COMMENT '所�?Cluster',
  `engine_id` varchar(128) DEFAULT NULL COMMENT 'docker engine id',
  `admin_ip` varchar(128) NOT NULL COMMENT '内网网卡 IP，UINT32',
  `room` varchar(128) DEFAULT NULL COMMENT '机房编号',
  `seat` varchar(128) DEFAULT NULL COMMENT '机架编号',
  `max_container` int(11) NOT NULL COMMENT '最大容器数�?,
  `status` int(4) unsigned NOT NULL DEFAULT '0' COMMENT '物理机状态\n0	准备入库			import\n1	初始化安装中		installing\n2	初始化安装成�?installed\n3	初始化安装失�?installfailed\n4	测试�?		testing\n5	测试失败			failedtest\n6	启用				enable\n7	停用				disable\n',
  `register_at` datetime DEFAULT NULL COMMENT '注册时间',
  `deregister_at` datetime DEFAULT NULL COMMENT '注销时间',
  PRIMARY KEY (`ai`),
  UNIQUE KEY `id_UNIQUE` (`id`),
  UNIQUE KEY `name_UNIQUE` (`name`),
  UNIQUE KEY `admin_ip_UNIQUE` (`admin_ip`)
) ENGINE=InnoDB AUTO_INCREMENT=86 DEFAULT CHARSET=utf8;
/*!40101 SET character_set_client = @saved_cs_client */;

--
-- Table structure for table `tbl_dbaas_port`
--

DROP TABLE IF EXISTS `tbl_dbaas_port`;
/*!40101 SET @saved_cs_client     = @@character_set_client */;
/*!40101 SET character_set_client = utf8 */;
CREATE TABLE `tbl_dbaas_port` (
  `ai` int(24) NOT NULL AUTO_INCREMENT COMMENT '自增字段，与业务无关',
  `port` int(11) unsigned NOT NULL COMMENT '端口号，10000+，分配给容器使用，容器的端口是唯一的�?,
  `name` varchar(128) DEFAULT NULL COMMENT '端口名称',
  `unit_id` varchar(128) DEFAULT NULL COMMENT '所属单元ID',
  `unit_name` varchar(128) DEFAULT NULL COMMENT '所属单元名�?,
  `proto` varchar(45) DEFAULT NULL COMMENT '协议类型 tcp �?udp',
  `allocated` tinyint(1) unsigned NOT NULL DEFAULT '0' COMMENT '是否被分配，0：未分配�?：已分配�?,
  PRIMARY KEY (`ai`),
  UNIQUE KEY `port_UNIQUE` (`port`)
) ENGINE=InnoDB AUTO_INCREMENT=202 DEFAULT CHARSET=utf8;
/*!40101 SET character_set_client = @saved_cs_client */;

--
-- Table structure for table `tbl_dbaas_raid_group`
--

DROP TABLE IF EXISTS `tbl_dbaas_raid_group`;
/*!40101 SET @saved_cs_client     = @@character_set_client */;
/*!40101 SET character_set_client = utf8 */;
CREATE TABLE `tbl_dbaas_raid_group` (
  `ai` int(24) NOT NULL AUTO_INCREMENT COMMENT '自增字段，与业务无关',
  `id` varchar(128) NOT NULL COMMENT '存储RAID ID',
  `storage_rg_id` int(11) NOT NULL COMMENT '在存储系统上的Raid group ID',
  `storage_system_id` varchar(128) NOT NULL COMMENT 'RAID GROUP 所属存储系统ID',
  `enabled` tinyint(1) unsigned NOT NULL DEFAULT '1' COMMENT '是否启用�?：停用，1：启用�?,
  PRIMARY KEY (`ai`),
  UNIQUE KEY `id_UNIQUE` (`storage_system_id`,`storage_rg_id`)
) ENGINE=InnoDB AUTO_INCREMENT=2 DEFAULT CHARSET=utf8;
/*!40101 SET character_set_client = @saved_cs_client */;

--
-- Table structure for table `tbl_dbaas_service`
--

DROP TABLE IF EXISTS `tbl_dbaas_service`;
/*!40101 SET @saved_cs_client     = @@character_set_client */;
/*!40101 SET character_set_client = utf8 */;
CREATE TABLE `tbl_dbaas_service` (
  `ai` int(24) NOT NULL AUTO_INCREMENT COMMENT '自增字段，与业务无关',
  `id` varchar(128) NOT NULL COMMENT '服务ID',
  `name` varchar(128) NOT NULL COMMENT '服务名称',
  `business_code` varchar(128) NOT NULL COMMENT '子系统代�?,
  `description` text NOT NULL COMMENT '服务描述信息',
  `architecture` varchar(128) NOT NULL COMMENT '服务结构描述:\n<软件名称>:<数量>#<软件名称>:<数量>\n例：switch_manager:1#proxy:1#upsql:2',
  `auto_healing` tinyint(1) unsigned DEFAULT '0' COMMENT '服务自动愈合�?:不启用，1：启�?,
  `auto_scaling` tinyint(1) unsigned DEFAULT '0' COMMENT '自动扩缩�?：不启用�?：启�?,
  `backup_max_size` bigint(128) unsigned DEFAULT NULL COMMENT '备份文件总大小，单位：byte',
  `backup_files_retention` bigint(128) DEFAULT NULL COMMENT '文件保存时间，单位是Hour',
  `status` tinyint(4) unsigned DEFAULT NULL COMMENT '管理状态\n0	已分配\n1	创建中\n2	启动中\n3	停止中\n4	删除中\n5	备份中\n6	还原中\n99	无任�?,
  `created_at` datetime NOT NULL COMMENT '创建日期',
  `finished_at` datetime DEFAULT NULL COMMENT '创建完成日期',
  PRIMARY KEY (`ai`),
  UNIQUE KEY `id_UNIQUE` (`id`),
  UNIQUE KEY `name_UNIQUE` (`name`)
) ENGINE=InnoDB AUTO_INCREMENT=81 DEFAULT CHARSET=utf8;
/*!40101 SET character_set_client = @saved_cs_client */;

--
-- Table structure for table `tbl_dbaas_storage_hitachi`
--

DROP TABLE IF EXISTS `tbl_dbaas_storage_hitachi`;
/*!40101 SET @saved_cs_client     = @@character_set_client */;
/*!40101 SET character_set_client = utf8 */;
CREATE TABLE `tbl_dbaas_storage_hitachi` (
  `ai` int(24) NOT NULL AUTO_INCREMENT COMMENT '自增字段，与业务无关',
  `id` varchar(128) NOT NULL COMMENT 'storage_system_ID',
  `vendor` varchar(128) NOT NULL COMMENT '厂商，HUAWEI / HITACHI',
  `admin_unit` varchar(128) NOT NULL COMMENT '管理域名称，HDS专有',
  `lun_start` int(11) unsigned NOT NULL COMMENT '起始位，HDS专有',
  `lun_end` int(11) unsigned NOT NULL COMMENT '结束位，HDS专有',
  `hlu_start` int(11) unsigned NOT NULL COMMENT 'host_lun_start ',
  `hlu_end` int(11) unsigned NOT NULL COMMENT 'host_un_end',
  PRIMARY KEY (`ai`),
  UNIQUE KEY `id_UNIQUE` (`id`)
) ENGINE=InnoDB AUTO_INCREMENT=2 DEFAULT CHARSET=utf8;
/*!40101 SET character_set_client = @saved_cs_client */;

--
-- Table structure for table `tbl_dbaas_storage_huawei`
--

DROP TABLE IF EXISTS `tbl_dbaas_storage_huawei`;
/*!40101 SET @saved_cs_client     = @@character_set_client */;
/*!40101 SET character_set_client = utf8 */;
CREATE TABLE `tbl_dbaas_storage_huawei` (
  `ai` int(24) NOT NULL AUTO_INCREMENT COMMENT '自增字段，与业务无关',
  `id` varchar(128) NOT NULL COMMENT 'storage_system_ID',
  `vendor` varchar(128) NOT NULL COMMENT '厂商，huawei / HDS',
  `ip_addr` varchar(45) NOT NULL COMMENT '管理IP，huawei专有',
  `username` varchar(45) NOT NULL COMMENT '用户名，huawei专有',
  `password` varchar(45) NOT NULL COMMENT '密码，huawei专有',
  `hlu_start` int(11) unsigned NOT NULL COMMENT 'host_lun_start ',
  `hlu_end` int(11) unsigned NOT NULL COMMENT 'host_un_end',
  PRIMARY KEY (`ai`),
  UNIQUE KEY `id_UNIQUE` (`id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8;
/*!40101 SET character_set_client = @saved_cs_client */;

--
-- Table structure for table `tbl_dbaas_system_config`
--

DROP TABLE IF EXISTS `tbl_dbaas_system_config`;
/*!40101 SET @saved_cs_client     = @@character_set_client */;
/*!40101 SET character_set_client = utf8 */;
CREATE TABLE `tbl_dbaas_system_config` (
  `ai` int(24) NOT NULL AUTO_INCREMENT COMMENT '自增字段，与业务无关',
  `dc_id` int(11) NOT NULL COMMENT '站点ID',
  `retry` tinyint(4) DEFAULT NULL COMMENT '资源分配失败重试次数',
  `consul_ip` varchar(128) NOT NULL COMMENT 'Consul server IP地址, 包含多个IP',
  `consul_port` int(10) unsigned NOT NULL COMMENT 'Consul Server HTTP Port',
  `consul_dc` varchar(45) NOT NULL COMMENT 'Consul DataCenter',
  `consul_token` varchar(45) DEFAULT NULL COMMENT 'Consul Token，可以为�?,
  `consul_wait_time` int(10) DEFAULT NULL,
  `docker_port` int(10) unsigned NOT NULL COMMENT 'docker Port',
  `plugin_port` int(6) NOT NULL COMMENT 'docker volume plugin port',
  `horus_agent_port` int(11) NOT NULL,
  `registry_os_username` varchar(45) NOT NULL COMMENT 'registry 操作系统用户',
  `registry_os_password` varchar(45) NOT NULL COMMENT 'registry 操作系统用户密码',
  `registry_domain` varchar(45) NOT NULL COMMENT 'registry 域名',
  `registry_ip` varchar(45) NOT NULL COMMENT 'registry IP地址',
  `registry_port` int(6) NOT NULL COMMENT 'registry 端口',
  `registry_username` varchar(45) NOT NULL COMMENT 'registry 用户�?,
  `registry_password` varchar(45) NOT NULL COMMENT 'registry 用户密码',
  `registry_email` varchar(128) NOT NULL COMMENT 'registry 邮箱',
  `registry_ca_crt` text NOT NULL COMMENT 'registry证书文件内容',
  `registry_token` varchar(4096) DEFAULT NULL,
  `source_dir` varchar(128) NOT NULL COMMENT '物理机初始化包绝对路�?,
  `destination_dir` varchar(128) NOT NULL COMMENT '物理机初始化包远程目标目�?,
  `init_script_name` varchar(45) NOT NULL COMMENT '物理机入库初始化脚本�?,
  `clean_script_name` varchar(45) NOT NULL COMMENT '物理机出库清理脚本名',
  `ca_crt_name` varchar(45) NOT NULL COMMENT '证书文件名称',
  `nfs_ip` varchar(45) NOT NULL COMMENT 'nfs IP地址',
  `nfs_dir` varchar(128) NOT NULL COMMENT 'nfs 源目�?,
  `nfs_mount_dir` varchar(128) NOT NULL COMMENT 'nfs 挂载目录',
  `nfs_mount_opts` varchar(128) DEFAULT NULL COMMENT 'nfs 挂载参数',
  `backup_dir` varchar(128) NOT NULL COMMENT '挂载到容器内的备份目�?,
  `check_username` varchar(128) NOT NULL,
  `check_password` varchar(128) NOT NULL,
  `mon_username` varchar(128) NOT NULL COMMENT '数据库监控用�?,
  `mon_password` varchar(128) NOT NULL COMMENT '数据库监控用户密�?,
  `repl_username` varchar(128) NOT NULL COMMENT '数据库数据复制用�?,
  `repl_password` varchar(128) NOT NULL COMMENT '数据库数据复制用户密�?,
  `cup_dba_username` varchar(128) NOT NULL COMMENT '数据库超级用�?,
  `cup_dba_password` varchar(128) NOT NULL COMMENT '数据库超级用户密�?,
  `db_username` varchar(128) NOT NULL COMMENT '数据库管理用户，用于proxy用户映射权限db',
  `db_password` varchar(128) NOT NULL,
  `ap_username` varchar(128) NOT NULL COMMENT '数据库应用用户，用于proxy用户映射权限ap',
  `ap_password` varchar(128) NOT NULL,
  PRIMARY KEY (`ai`),
  UNIQUE KEY `dc_id_UNIQUE` (`dc_id`)
) ENGINE=InnoDB AUTO_INCREMENT=2 DEFAULT CHARSET=utf8;
/*!40101 SET character_set_client = @saved_cs_client */;

--
-- Table structure for table `tbl_dbaas_task`
--

DROP TABLE IF EXISTS `tbl_dbaas_task`;
/*!40101 SET @saved_cs_client     = @@character_set_client */;
/*!40101 SET character_set_client = utf8 */;
CREATE TABLE `tbl_dbaas_task` (
  `ai` int(24) NOT NULL AUTO_INCREMENT COMMENT '自增字段，与业务无关',
  `id` varchar(128) NOT NULL COMMENT '任务ID',
  `name` varchar(128) NOT NULL,
  `related` varchar(128) NOT NULL COMMENT '关联表名称或者对象�?,
  `link_to` varchar(128) NOT NULL COMMENT '关联ID',
  `description` varchar(128) DEFAULT NULL COMMENT '描述',
  `labels` varchar(512) DEFAULT NULL,
  `errors` longtext,
  `timeout` int(11) unsigned DEFAULT NULL,
  `status` tinyint(4) unsigned DEFAULT '0' COMMENT '任务状态\n0	创建任务				create\n1	执行�?	running		\n2	任务中止,未完�?	stop\n3	任务未执行，被取�?	cancel\n4	任务完成	done\n5	任务超时				timeout\n6	任务失败				faile',
  `created_at` datetime NOT NULL COMMENT '创建时间',
  `finished_at` datetime DEFAULT NULL COMMENT '完成时间',
  `timestamp` bigint(128) DEFAULT NULL COMMENT '时间�?,
  PRIMARY KEY (`ai`),
  UNIQUE KEY `id_UNIQUE` (`id`)
) ENGINE=InnoDB AUTO_INCREMENT=340 DEFAULT CHARSET=utf8;
/*!40101 SET character_set_client = @saved_cs_client */;

--
-- Table structure for table `tbl_dbaas_unit`
--

DROP TABLE IF EXISTS `tbl_dbaas_unit`;
/*!40101 SET @saved_cs_client     = @@character_set_client */;
/*!40101 SET character_set_client = utf8 */;
CREATE TABLE `tbl_dbaas_unit` (
  `ai` int(24) NOT NULL AUTO_INCREMENT COMMENT '自增字段，与业务无关',
  `id` varchar(128) NOT NULL COMMENT 'unit ID',
  `name` varchar(128) NOT NULL COMMENT 'unit 名称，命名规则为<unit_id_8bit>_<service_name>',
  `type` varchar(45) NOT NULL COMMENT 'unit 类型�?switch_manager / upproxy / upsql ',
  `service_id` varchar(128) NOT NULL COMMENT ' 所�?Service ID',
  `image_id` varchar(128) NOT NULL COMMENT '关联的镜像ID',
  `image_name` varchar(128) NOT NULL COMMENT '镜像名称，命名规则为<software_name>_<version>',
  `network_mode` varchar(45) CHARACTER SET utf8 COLLATE utf8_bin DEFAULT 'host' COMMENT '网络模式，默�?host',
  `node_id` varchar(128) DEFAULT NULL COMMENT '所在主机ID',
  `container_id` varchar(128) DEFAULT NULL COMMENT 'docker 生成的ID',
  `unit_config_id` varchar(128) NOT NULL COMMENT '配置文件ID',
  `check_interval` int(11) unsigned DEFAULT NULL COMMENT '服务检查间隔时�?单位为秒',
  `status` int(11) unsigned NOT NULL COMMENT '管理状态\n0	已分配\n1	创建中\n2	启动中\n3	停止中\n4	迁移中\n5	重建中\n6	删除中\n7	备份中\n8	还原中\n99	无任�?,
  `created_at` datetime NOT NULL,
  `latest_error` longtext COMMENT '最新错误信�?,
  PRIMARY KEY (`ai`),
  UNIQUE KEY `id_UNIQUE` (`id`)
) ENGINE=InnoDB AUTO_INCREMENT=385 DEFAULT CHARSET=utf8;
/*!40101 SET character_set_client = @saved_cs_client */;

--
-- Table structure for table `tbl_dbaas_unit_config`
--

DROP TABLE IF EXISTS `tbl_dbaas_unit_config`;
/*!40101 SET @saved_cs_client     = @@character_set_client */;
/*!40101 SET character_set_client = utf8 */;
CREATE TABLE `tbl_dbaas_unit_config` (
  `ai` int(24) NOT NULL AUTO_INCREMENT COMMENT '自增字段，与业务无关',
  `id` varchar(128) NOT NULL COMMENT '配置文件ID',
  `image_id` varchar(128) NOT NULL COMMENT '镜像软件ID',
  `version` int(11) NOT NULL COMMENT '版本号\n�? 开始，更新一次＋1',
  `parent_id` varchar(128) DEFAULT NULL COMMENT '前一次的配置文件ID',
  `content` longtext NOT NULL COMMENT '配置文件内容',
  `config_file_path` varchar(128) NOT NULL COMMENT '文件路径',
  `created_at` datetime NOT NULL COMMENT '创建时间',
  `unit_id` varchar(128) DEFAULT NULL,
  PRIMARY KEY (`ai`),
  UNIQUE KEY `id_UNIQUE` (`id`)
) ENGINE=InnoDB AUTO_INCREMENT=352 DEFAULT CHARSET=utf8;
/*!40101 SET character_set_client = @saved_cs_client */;

--
-- Table structure for table `tbl_dbaas_users`
--

DROP TABLE IF EXISTS `tbl_dbaas_users`;
/*!40101 SET @saved_cs_client     = @@character_set_client */;
/*!40101 SET character_set_client = utf8 */;
CREATE TABLE `tbl_dbaas_users` (
  `ai` int(24) NOT NULL AUTO_INCREMENT COMMENT '自增字段，与业务无关',
  `id` varchar(128) NOT NULL COMMENT '数据库用户ID',
  `service_id` varchar(128) NOT NULL COMMENT '所属服务ID',
  `type` varchar(45) NOT NULL COMMENT '用户类型\nupsql        数据库\nupproxy	 代理',
  `username` varchar(128) NOT NULL COMMENT '用户�?,
  `password` varchar(128) DEFAULT NULL COMMENT '密码，暂时使用明文密�?,
  `role` varchar(45) NOT NULL COMMENT '用户权限角色，用于对应数据库用户，角色名称为数据库用户名\ncup_db\ndb\nap',
  `blacklist` varchar(1024) DEFAULT NULL COMMENT '黑名�?,
  `whitelist` varchar(1024) DEFAULT NULL COMMENT '白名�?,
  `created_at` datetime NOT NULL,
  `read_only` tinyint(3) DEFAULT '0' COMMENT '只读权限',
  PRIMARY KEY (`ai`),
  UNIQUE KEY `id_UNIQUE` (`id`)
) ENGINE=InnoDB AUTO_INCREMENT=561 DEFAULT CHARSET=utf8;
/*!40101 SET character_set_client = @saved_cs_client */;

--
-- Table structure for table `tbl_dbaas_volumes`
--

DROP TABLE IF EXISTS `tbl_dbaas_volumes`;
/*!40101 SET @saved_cs_client     = @@character_set_client */;
/*!40101 SET character_set_client = utf8 */;
CREATE TABLE `tbl_dbaas_volumes` (
  `ai` int(24) NOT NULL AUTO_INCREMENT COMMENT '自增字段，与业务无关',
  `id` varchar(128) NOT NULL COMMENT 'volume ID',
  `name` varchar(128) NOT NULL COMMENT '名称',
  `size` bigint(128) NOT NULL COMMENT 'volume 容量大小，单位byte',
  `VGname` varchar(128) NOT NULL COMMENT '所属Volume Group',
  `driver` varchar(45) NOT NULL COMMENT 'docker plugin 驱动名称',
  `fstype` varchar(45) NOT NULL COMMENT '文件系统类型',
  `unit_id` varchar(128) NOT NULL COMMENT '所属Unit ID',
  PRIMARY KEY (`ai`),
  UNIQUE KEY `id_UNIQUE` (`id`)
) ENGINE=InnoDB AUTO_INCREMENT=922 DEFAULT CHARSET=utf8;
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

-- Dump completed on 2016-12-20 10:14:21
