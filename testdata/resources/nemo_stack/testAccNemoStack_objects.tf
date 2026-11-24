resource "nemo_stack" "test" {
  name             = "%[1]s"
  target           = { orchestrator = "%[2]s" }
  parameters       = { nemo-timeout = "%[3]s" }
  credentials      = { greeting = { env = "TF_ACC_NEMO_GREETING" } }
  purge_on_destroy = true
  installations = [
    { name = "hello", file = "%[4]s", override = %[5]s },
    %[6]s
  ]
}
