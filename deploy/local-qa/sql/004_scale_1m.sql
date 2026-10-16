SET NOCOUNT ON;
IF DB_NAME() <> N'DataFlowFixtureQA' THROW 51000, 'Refusing non-QA database', 1;
IF OBJECT_ID(N'Erp.PartTranScaleM') IS NULL
CREATE TABLE Erp.PartTranScaleM(
 Company nvarchar(8) NOT NULL, TranNum bigint NOT NULL, PartNum nvarchar(50) NOT NULL,
 TranDate date NOT NULL, SysDate datetime2(3) NOT NULL, TranType nvarchar(16) NOT NULL,
 TranQty decimal(18,6) NULL, ExtCost decimal(18,4) NULL, CommentText nvarchar(200) NULL,
 SysRowID uniqueidentifier NOT NULL, SysRevID rowversion NOT NULL,
 CONSTRAINT PK_PartTranScaleM PRIMARY KEY(Company,TranNum), CONSTRAINT UQ_PartTranScaleM_SysRowID UNIQUE(SysRowID));
IF NOT EXISTS(SELECT 1 FROM qa.fixture_manifest WHERE dataset=N'SCALE_M')
BEGIN TRY
 BEGIN TRANSACTION;
 ;WITH n AS (SELECT TOP (1000000) ROW_NUMBER() OVER(ORDER BY (SELECT NULL)) n FROM sys.all_objects a CROSS JOIN sys.all_objects b)
 INSERT Erp.PartTranScaleM(Company,TranNum,PartNum,TranDate,SysDate,TranType,TranQty,ExtCost,CommentText,SysRowID)
 SELECT CASE WHEN n%2=0 THEN N'VN01' ELSE N'US01' END,n,CONCAT(N'P-',RIGHT(CONCAT('000000',n%997),6)),DATEADD(day,n%730,CONVERT(date,'2024-01-01')),DATEADD(second,n,CONVERT(datetime2,'2024-01-01')),
  CASE n%4 WHEN 0 THEN N'RECEIPT' WHEN 1 THEN N'ISSUE' WHEN 2 THEN N'ADJUST' ELSE N'SHIP' END,
  CASE WHEN n%101=0 THEN NULL WHEN n%17=0 THEN -CONVERT(decimal(18,6),n)/10 ELSE CONVERT(decimal(18,6),n)/10 END,
  CASE WHEN n%137=0 THEN NULL ELSE CONVERT(decimal(18,4),n)*1.2345 END,
  CASE WHEN n%113=0 THEN N'Kiểm thử tiếng Việt' ELSE CONCAT(N'scale-m-',n) END,
  CONVERT(uniqueidentifier,HASHBYTES('MD5',CONCAT('20261016-ScaleM-',n))) FROM n;
 INSERT qa.fixture_manifest(dataset,expected_rows,seed_version) VALUES(N'SCALE_M',1000000,20261016);
 COMMIT TRANSACTION;
END TRY
BEGIN CATCH
 IF @@TRANCOUNT>0 ROLLBACK TRANSACTION;
 THROW;
END CATCH;
GO
