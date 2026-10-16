SET NOCOUNT ON;
IF DB_NAME() <> N'DataFlowFixtureQA' THROW 51000, 'Refusing non-QA database', 1;
IF (SELECT COUNT(*) FROM sys.tables t JOIN sys.schemas s ON s.schema_id=t.schema_id WHERE s.name='Erp') < 4 THROW 51000, 'Fixture schema missing', 1;
IF NOT EXISTS(SELECT 1 FROM qa.fixture_manifest WHERE dataset=N'CORRECTNESS')
BEGIN
 BEGIN TRY
  BEGIN TRANSACTION;
  ;WITH n AS (SELECT TOP (10000) ROW_NUMBER() OVER(ORDER BY (SELECT NULL)) n FROM sys.all_objects a CROSS JOIN sys.all_objects b)
  INSERT Erp.PartTran(Company,TranNum,PartNum,TranDate,SysDate,TranType,TranQty,ExtCost,CommentText,SysRowID)
  SELECT CASE WHEN n%2=0 THEN N'VN01' ELSE N'US01' END,n,CONCAT(N'P-',RIGHT(CONCAT('000000',n%997),6)),DATEADD(day,n%730,CONVERT(date,'2024-01-01')),DATEADD(second,n,CONVERT(datetime2,'2024-01-01')),
    CASE n%4 WHEN 0 THEN N'RECEIPT' WHEN 1 THEN N'ISSUE' WHEN 2 THEN N'ADJUST' ELSE N'SHIP' END,
    CASE WHEN n%101=0 THEN NULL WHEN n%17=0 THEN -CONVERT(decimal(18,6),n)/10 ELSE CONVERT(decimal(18,6),n)/10 END,
    CASE WHEN n%137=0 THEN NULL ELSE CONVERT(decimal(18,4),n)*1.2345 END,
    CASE WHEN n%113=0 THEN N'Kiểm thử tiếng Việt' ELSE CONCAT(N'fixture-',n) END,
    CONVERT(uniqueidentifier,HASHBYTES('MD5',CONCAT('20261016-PartTran-',n))) FROM n;
  ;WITH n AS (SELECT TOP (200) ROW_NUMBER() OVER(ORDER BY (SELECT NULL)) n FROM sys.all_objects)
  INSERT Erp.Part(Company,PartNum,PartDescription,SysRowID) SELECT N'VN01',CONCAT(N'P-',RIGHT(CONCAT('000000',n),6)),CASE WHEN n%20=0 THEN NULL ELSE CONCAT(N'Linh kiện số ',n) END,CONVERT(uniqueidentifier,HASHBYTES('MD5',CONCAT('20261016-Part-',n))) FROM n;
  ;WITH n AS (SELECT TOP (100) ROW_NUMBER() OVER(ORDER BY (SELECT NULL)) n FROM sys.all_objects)
  INSERT Erp.InvcHead(Company,InvoiceNum,InvoiceDate,CustomerName,SysRowID) SELECT N'VN01',n,DATEADD(day,n,CONVERT(date,'2025-01-01')),CONCAT(N'Khách hàng ',n),CONVERT(uniqueidentifier,HASHBYTES('MD5',CONCAT('20261016-Head-',n))) FROM n;
  ;WITH n AS (SELECT TOP (300) ROW_NUMBER() OVER(ORDER BY (SELECT NULL)) n FROM sys.all_objects)
  INSERT Erp.InvcDtl(Company,InvoiceNum,InvoiceLine,PartNum,OrderQty,UnitPrice,SysRowID) SELECT N'VN01',((n-1)/3)+1,((n-1)%3)+1,CONCAT(N'P-',RIGHT(CONCAT('000000',n%200+1),6)),CONVERT(decimal(18,6),n)/3,CONVERT(decimal(18,4),n)*2.3456,CONVERT(uniqueidentifier,HASHBYTES('MD5',CONCAT('20261016-Dtl-',n))) FROM n;
  INSERT qa.fixture_manifest(dataset,expected_rows,seed_version) VALUES(N'CORRECTNESS',10000,20261016);
  COMMIT TRANSACTION;
 END TRY
 BEGIN CATCH
  IF @@TRANCOUNT > 0 ROLLBACK TRANSACTION;
  THROW;
 END CATCH
END;
GO

