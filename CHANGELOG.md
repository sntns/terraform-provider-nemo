# Changelog

## 0.1.0 (2026-10-29)


### Features

* deploy a bundle rebuilt at the same path, followed by content_hash ([7e7fdd0](https://github.com/sntns/terraform-provider-nemo/commit/7e7fdd0b0af53c48ec6fb37426a89edc96089c14))
* deploy, read, import and destroy a stack with nemo ([d99ce36](https://github.com/sntns/terraform-provider-nemo/commit/d99ce36640f875dea3005502405a678bb93a93ee))
* give a credential by value_wo, which neither the plan nor the state keeps ([c03008a](https://github.com/sntns/terraform-provider-nemo/commit/c03008ab68352a22f9f4985312e0da232346d7ff))
* import a stack nemo deployed, and bound its operations with timeouts ([a941662](https://github.com/sntns/terraform-provider-nemo/commit/a9416622e9d669c003210274afa68c18582453a1))
* read each part of a target's TLS from a source, as nemo's manifest does ([ee51386](https://github.com/sntns/terraform-provider-nemo/commit/ee51386e17556897d092a1c34bcc0209581e8924))
* read the records of a stack on the storage of its target's records type, with nemo 0.6.0 ([d0b86e6](https://github.com/sntns/terraform-provider-nemo/commit/d0b86e604ef32653f52c97921a72c70c095b0796))
* show the stack's manifest with its values redacted ([6f72474](https://github.com/sntns/terraform-provider-nemo/commit/6f72474386adace8af615d0421092bae2352a190))
* the schema of nemo_stack, before its actions ([30a8e16](https://github.com/sntns/terraform-provider-nemo/commit/30a8e164ec21156e585bcd00dd53f547a5e2e78e))
* write the manifest's paths absolute, from where Terraform runs ([925c1c8](https://github.com/sntns/terraform-provider-nemo/commit/925c1c83a519a4615fa7c941671bf194b330df10))


### Bug Fixes

* record an apply that its timeouts cut short, with nemo 0.5.1 ([b2112c0](https://github.com/sntns/terraform-provider-nemo/commit/b2112c0198d8c2592e5360eb09ec3c9e75b5ea6e))
