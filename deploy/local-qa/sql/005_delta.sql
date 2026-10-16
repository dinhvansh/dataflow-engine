SET NOCOUNT ON;
IF DB_NAME() <> N'DataFlowFixtureQA' THROW 51000, 'Refusing non-QA database', 1;
IF OBJECT_ID(N'qa.control_events') IS NULL CREATE TABLE qa.control_events(event_id int IDENTITY PRIMARY KEY,scenario nvarchar(30),key_value bigint,event_type nvarchar(10),created_at datetime2 DEFAULT sysdatetime());
BEGIN TRY
 BEGIN TRANSACTION;
 UPDATE Erp.PartTran SET TranQty=TranQty+100,ExtCost=ExtCost+10,CommentText=N'Cập nhật delta' OUTPUT N'DELTA_1',inserted.TranNum,N'UPDATE' INTO qa.control_events(scenario,key_value,event_type) WHERE TranNum BETWEEN 1 AND 5;
 ;WITH n AS (SELECT TOP (10) ROW_NUMBER() OVER(ORDER BY (SELECT NULL))+10000 n FROM sys.all_objects)
 INSERT Erp.PartTran(Company,TranNum,PartNum,TranDate,SysDate,TranType,TranQty,ExtCost,CommentText,SysRowID)
 OUTPUT N'DELTA_1',inserted.TranNum,N'INSERT' INTO qa.control_events(scenario,key_value,event_type)
 SELECT N'VN01',n,CONCAT(N'P-D',n),CONVERT(date,'2026-10-16'),DATEADD(second,n-10000,CONVERT(datetime2,'2026-10-16')),N'ADJUST',n/10.0,n*1.25,N'Thêm mới',CONVERT(uniqueidentifier,HASHBYTES('MD5',CONCAT('20261016-Delta-',n))) FROM n;
 DELETE FROM Erp.PartTran OUTPUT N'DELTA_1',deleted.TranNum,N'DELETE' INTO qa.control_events(scenario,key_value,event_type) WHERE TranNum=10;
 COMMIT TRANSACTION;
END TRY
BEGIN CATCH
 IF @@TRANCOUNT>0 ROLLBACK TRANSACTION;
 THROW;
END CATCH;
GO
