IF DB_ID(N'DataFlowFixtureQA') IS NULL CREATE DATABASE DataFlowFixtureQA;
GO
ALTER DATABASE DataFlowFixtureQA SET ALLOW_SNAPSHOT_ISOLATION ON;
GO
USE DataFlowFixtureQA;
GO
IF SCHEMA_ID(N'Erp') IS NULL EXEC(N'CREATE SCHEMA Erp');
IF SCHEMA_ID(N'qa') IS NULL EXEC(N'CREATE SCHEMA qa');
GO
IF OBJECT_ID(N'Erp.PartTran') IS NULL
CREATE TABLE Erp.PartTran(
  Company nvarchar(8) NOT NULL, TranNum bigint NOT NULL,
  PartNum nvarchar(50) NOT NULL, TranDate date NOT NULL, SysDate datetime2(3) NOT NULL,
  TranType nvarchar(16) NOT NULL, TranQty decimal(18,6) NULL, ExtCost decimal(18,4) NULL,
  CommentText nvarchar(200) NULL, SysRowID uniqueidentifier NOT NULL,
  SysRevID rowversion NOT NULL, CONSTRAINT PK_PartTran PRIMARY KEY(Company,TranNum),
  CONSTRAINT UQ_PartTran_SysRowID UNIQUE(SysRowID));
IF OBJECT_ID(N'Erp.Part') IS NULL
CREATE TABLE Erp.Part(Company nvarchar(8) NOT NULL,PartNum nvarchar(50) NOT NULL,PartDescription nvarchar(200) NULL,SysRowID uniqueidentifier NOT NULL,SysRevID rowversion NOT NULL,CONSTRAINT PK_Part PRIMARY KEY(Company,PartNum),CONSTRAINT UQ_Part_SysRowID UNIQUE(SysRowID));
IF OBJECT_ID(N'Erp.InvcHead') IS NULL
CREATE TABLE Erp.InvcHead(Company nvarchar(8) NOT NULL,InvoiceNum int NOT NULL,InvoiceDate date NOT NULL,CustomerName nvarchar(100) NULL,SysRowID uniqueidentifier NOT NULL,SysRevID rowversion NOT NULL,CONSTRAINT PK_InvcHead PRIMARY KEY(Company,InvoiceNum),CONSTRAINT UQ_InvcHead_SysRowID UNIQUE(SysRowID));
IF OBJECT_ID(N'Erp.InvcDtl') IS NULL
CREATE TABLE Erp.InvcDtl(Company nvarchar(8) NOT NULL,InvoiceNum int NOT NULL,InvoiceLine int NOT NULL,PartNum nvarchar(50) NOT NULL,OrderQty decimal(18,6) NULL,UnitPrice decimal(18,4) NULL,SysRowID uniqueidentifier NOT NULL,SysRevID rowversion NOT NULL,CONSTRAINT PK_InvcDtl PRIMARY KEY(Company,InvoiceNum,InvoiceLine),CONSTRAINT UQ_InvcDtl_SysRowID UNIQUE(SysRowID));
IF OBJECT_ID(N'qa.fixture_manifest') IS NULL CREATE TABLE qa.fixture_manifest(dataset nvarchar(30) PRIMARY KEY,expected_rows bigint NOT NULL,seed_version int NOT NULL,created_at datetime2 NOT NULL DEFAULT sysdatetime());
GO
IF NOT EXISTS(SELECT 1 FROM sys.sql_logins WHERE name=N'qa_seed_admin') EXEC(N'CREATE LOGIN qa_seed_admin WITH PASSWORD='''+'$(SeedPassword)'+''', CHECK_POLICY=OFF');
IF NOT EXISTS(SELECT 1 FROM sys.sql_logins WHERE name=N'qa_erp_reader') EXEC(N'CREATE LOGIN qa_erp_reader WITH PASSWORD='''+'$(ReaderPassword)'+''', CHECK_POLICY=OFF');
IF USER_ID(N'qa_seed_admin') IS NULL CREATE USER qa_seed_admin FOR LOGIN qa_seed_admin;
IF USER_ID(N'qa_erp_reader') IS NULL CREATE USER qa_erp_reader FOR LOGIN qa_erp_reader;
ALTER ROLE db_datareader ADD MEMBER qa_seed_admin; ALTER ROLE db_datawriter ADD MEMBER qa_seed_admin;
GRANT EXECUTE, CONTROL ON SCHEMA::qa TO qa_seed_admin; GRANT SELECT ON SCHEMA::Erp TO qa_erp_reader; DENY INSERT,UPDATE,DELETE,ALTER ON SCHEMA::Erp TO qa_erp_reader;
GO

