-- LARGE FIXTURE, NOT ERP: explicit operator opt-in via run-core-7m.sh.
-- This script is deliberately incompatible with ANY other database.
SET NOCOUNT ON;
IF DB_NAME() <> N'DataFlowFixtureQA'
 THROW 51000,'Refusing any database other than DataFlowFixtureQA',1;
IF ORIGINAL_LOGIN() NOT IN (N'sa',N'qa_seed_admin')
 THROW 51000,'7m QA seeding requires disposable fixture admin only',1;
IF OBJECT_ID(N'qa.fixture_manifest') IS NULL
 THROW 51000,'Disposable seed manifest missing',1;
IF OBJECT_ID(N'Erp.PartTranScale7M') IS NULL
BEGIN
 CREATE TABLE Erp.PartTranScale7M(
  Company nvarchar(8) NOT NULL,TranNum bigint NOT NULL,
  PartNum nvarchar(50) NOT NULL,TranDate date NOT NULL,
  SysDate datetime2(3) NOT NULL,TranType nvarchar(16) NOT NULL,
  TranQty decimal(18,6) NULL,ExtCost decimal(18,4) NULL,
  CommentText nvarchar(200) NULL,SysRowID uniqueidentifier NOT NULL,
  SysRevID rowversion NOT NULL,
  CONSTRAINT PK_PartTranScale7M PRIMARY KEY(Company,TranNum)
 );
END;
DECLARE @next bigint=(SELECT ISNULL(MAX(TranNum),0) FROM Erp.PartTranScale7M);
-- Fixture-only bulk preparation: maintaining a random UUID index row by row
-- thrashes HDD-backed disposable volumes. Resume committed batches, then
-- rebuild the same UNIQUE constraint before exposing the fixture to readers.
-- The primary key stays enforced throughout. No benchmark assertion changes.
IF @next < 7000000 AND EXISTS (
 SELECT 1 FROM sys.key_constraints WHERE parent_object_id=OBJECT_ID(N'Erp.PartTranScale7M')
 AND name=N'UQ_PartTranScale7M_SysRowID'
)
 ALTER TABLE Erp.PartTranScale7M DROP CONSTRAINT UQ_PartTranScale7M_SysRowID;
WHILE @next < 7000000
BEGIN
 DECLARE @batch int=CASE WHEN 7000000-@next<50000
  THEN CONVERT(int,7000000-@next) ELSE 50000 END;
 BEGIN TRY
  BEGIN TRANSACTION;
  ;WITH n AS (
   SELECT TOP (@batch) ROW_NUMBER() OVER(ORDER BY (SELECT NULL)) AS rn
   FROM sys.all_objects a CROSS JOIN sys.all_objects b
  )
  INSERT Erp.PartTranScale7M(Company,TranNum,PartNum,TranDate,SysDate,
   TranType,TranQty,ExtCost,CommentText,SysRowID)
  SELECT CASE WHEN (@next+rn)%2=0 THEN N'VN01' ELSE N'US01' END,
   @next+rn,
   CONCAT(N'P-',RIGHT(CONCAT('000000',(@next+rn)%997),6)),
   DATEADD(day,CONVERT(int,(@next+rn)%730),CONVERT(date,'2024-01-01')),
   DATEADD(second,CONVERT(int,(@next+rn)%86400),CONVERT(datetime2,'2024-01-01')),
   CASE (@next+rn)%4 WHEN 0 THEN N'RECEIPT' WHEN 1 THEN N'ISSUE'
      WHEN 2 THEN N'ADJUST' ELSE N'SHIP' END,
   CONVERT(decimal(18,6),@next+rn)/10,
   CONVERT(decimal(18,4),@next+rn)*1.2345,
   CASE WHEN (@next+rn)%113=0 THEN N'Kiểm thử tiếng Việt'
     ELSE CONCAT(N'qa-seven-million-',@next+rn) END,
   CONVERT(uniqueidentifier,HASHBYTES('MD5',
     CONCAT('DATAFLOW-ISOLATED-7M-',@next+rn)))
  FROM n;
  COMMIT TRANSACTION;
  SET @next=@next+@batch;
 END TRY
 BEGIN CATCH
  IF @@TRANCOUNT>0 ROLLBACK TRANSACTION;
  THROW;
 END CATCH;
END;
IF NOT EXISTS (
 SELECT 1 FROM sys.key_constraints WHERE parent_object_id=OBJECT_ID(N'Erp.PartTranScale7M')
 AND name=N'UQ_PartTranScale7M_SysRowID'
)
 ALTER TABLE Erp.PartTranScale7M ADD CONSTRAINT UQ_PartTranScale7M_SysRowID UNIQUE(SysRowID);
-- The managed reader pages by (SysRevID, SysRowID). Build this after seeding
-- so fixture creation stays fast and each 10k-row page can seek by cursor.
IF NOT EXISTS (
 SELECT 1 FROM sys.indexes
 WHERE object_id=OBJECT_ID(N'Erp.PartTranScale7M')
   AND name=N'IX_PartTranScale7M_SysRevID_SysRowID'
)
 CREATE INDEX IX_PartTranScale7M_SysRevID_SysRowID
 ON Erp.PartTranScale7M(SysRevID,SysRowID)
 INCLUDE (Company,TranNum,PartNum,TranDate,SysDate,TranType,
          TranQty,ExtCost,CommentText);
IF (SELECT COUNT_BIG(*) FROM Erp.PartTranScale7M)<>7000000
 THROW 51000,'QA 7m fixture count mismatch',1;
IF NOT EXISTS(SELECT 1 FROM qa.fixture_manifest WHERE dataset=N'SCALE_7M')
 INSERT qa.fixture_manifest(dataset,expected_rows,seed_version)
 VALUES(N'SCALE_7M',7000000,20261018);
IF EXISTS(SELECT 1 FROM qa.fixture_manifest WHERE dataset=N'SCALE_7M'
 AND expected_rows<>7000000)
 THROW 51000,'QA manifest tampered',1;
SELECT N'SCALE_7M' AS dataset, COUNT_BIG(*) AS rows_ready
 FROM Erp.PartTranScale7M;
GO
