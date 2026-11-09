import dagster as dg


@dg.asset
def numbers():
    return [1, 2, 3]


@dg.asset
def total(numbers):
    return sum(numbers)


everything = dg.define_asset_job("everything", selection="*")

defs = dg.Definitions(
    assets=[numbers, total],
    jobs=[everything],
    schedules=[
        dg.ScheduleDefinition(
            job=everything,
            cron_schedule="* * * * *",
            default_status=dg.DefaultScheduleStatus.RUNNING,
        )
    ],
)
