resource "nemo_stack" "test" {
  name        = "%[1]s"
  target      = { orchestrator = "compose" }
  credentials = { greeting = { %[3]s } }
  installations = [
    {
      name     = "hello"
      file     = "%[2]s"
      override = <<-EOT
        services:
          web:
            secrets: [greeting]
        secrets:
          greeting:
            environment: GREETING
      EOT
    },
  ]
}
