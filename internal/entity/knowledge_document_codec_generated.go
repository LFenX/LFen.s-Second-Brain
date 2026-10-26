package entity

func (value *KnowledgeDocumentOriginalRef) UnmarshalJSON(raw []byte) error {
	type plain KnowledgeDocumentOriginalRef
	var decoded plain
	if err := knowledgeDocumentDecode("OriginalRef", raw, &decoded); err != nil {
		return err
	}
	*value = KnowledgeDocumentOriginalRef(decoded)
	return nil
}

func (value KnowledgeDocumentOriginalRef) MarshalJSON() ([]byte, error) {
	type plain KnowledgeDocumentOriginalRef
	return knowledgeDocumentMarshal("OriginalRef", plain(value))
}

func (value *KnowledgeDocumentSourceSnapshot) UnmarshalJSON(raw []byte) error {
	type plain KnowledgeDocumentSourceSnapshot
	var decoded plain
	if err := knowledgeDocumentDecode("SourceSnapshot", raw, &decoded); err != nil {
		return err
	}
	*value = KnowledgeDocumentSourceSnapshot(decoded)
	return nil
}

func (value KnowledgeDocumentSourceSnapshot) MarshalJSON() ([]byte, error) {
	type plain KnowledgeDocumentSourceSnapshot
	return knowledgeDocumentMarshal("SourceSnapshot", plain(value))
}

func (value *KnowledgeDocumentConfiguration) UnmarshalJSON(raw []byte) error {
	type plain KnowledgeDocumentConfiguration
	var decoded plain
	if err := knowledgeDocumentDecode("ProcessingConfiguration", raw, &decoded); err != nil {
		return err
	}
	*value = KnowledgeDocumentConfiguration(decoded)
	return nil
}

func (value KnowledgeDocumentConfiguration) MarshalJSON() ([]byte, error) {
	type plain KnowledgeDocumentConfiguration
	return knowledgeDocumentMarshal("ProcessingConfiguration", plain(value))
}

func (value *KnowledgeDocumentProducer) UnmarshalJSON(raw []byte) error {
	type plain KnowledgeDocumentProducer
	var decoded plain
	if err := knowledgeDocumentDecode("Producer", raw, &decoded); err != nil {
		return err
	}
	*value = KnowledgeDocumentProducer(decoded)
	return nil
}

func (value KnowledgeDocumentProducer) MarshalJSON() ([]byte, error) {
	type plain KnowledgeDocumentProducer
	return knowledgeDocumentMarshal("Producer", plain(value))
}

func (value *KnowledgeDocumentProfile) UnmarshalJSON(raw []byte) error {
	type plain KnowledgeDocumentProfile
	var decoded plain
	if err := knowledgeDocumentDecode("Profile", raw, &decoded); err != nil {
		return err
	}
	*value = KnowledgeDocumentProfile(decoded)
	return nil
}

func (value KnowledgeDocumentProfile) MarshalJSON() ([]byte, error) {
	type plain KnowledgeDocumentProfile
	return knowledgeDocumentMarshal("Profile", plain(value))
}

func (value *KnowledgeDocumentLimits) UnmarshalJSON(raw []byte) error {
	type plain KnowledgeDocumentLimits
	var decoded plain
	if err := knowledgeDocumentDecode("Limits", raw, &decoded); err != nil {
		return err
	}
	*value = KnowledgeDocumentLimits(decoded)
	return nil
}

func (value KnowledgeDocumentLimits) MarshalJSON() ([]byte, error) {
	type plain KnowledgeDocumentLimits
	return knowledgeDocumentMarshal("Limits", plain(value))
}

func (value *KnowledgeDocumentCapabilities) UnmarshalJSON(raw []byte) error {
	type plain KnowledgeDocumentCapabilities
	var decoded plain
	if err := knowledgeDocumentDecode("Capabilities", raw, &decoded); err != nil {
		return err
	}
	*value = KnowledgeDocumentCapabilities(decoded)
	return nil
}

func (value KnowledgeDocumentCapabilities) MarshalJSON() ([]byte, error) {
	type plain KnowledgeDocumentCapabilities
	return knowledgeDocumentMarshal("Capabilities", plain(value))
}

func (value *KnowledgeDocumentPolicy) UnmarshalJSON(raw []byte) error {
	type plain KnowledgeDocumentPolicy
	var decoded plain
	if err := knowledgeDocumentDecode("DocumentPolicy", raw, &decoded); err != nil {
		return err
	}
	*value = KnowledgeDocumentPolicy(decoded)
	return nil
}

func (value KnowledgeDocumentPolicy) MarshalJSON() ([]byte, error) {
	type plain KnowledgeDocumentPolicy
	return knowledgeDocumentMarshal("DocumentPolicy", plain(value))
}

func (value *KnowledgeDocumentPolicyChangeRequest) UnmarshalJSON(raw []byte) error {
	type plain KnowledgeDocumentPolicyChangeRequest
	var decoded plain
	if err := knowledgeDocumentDecode("PolicyChangeRequest", raw, &decoded); err != nil {
		return err
	}
	*value = KnowledgeDocumentPolicyChangeRequest(decoded)
	return nil
}

func (value KnowledgeDocumentPolicyChangeRequest) MarshalJSON() ([]byte, error) {
	type plain KnowledgeDocumentPolicyChangeRequest
	return knowledgeDocumentMarshal("PolicyChangeRequest", plain(value))
}

func (value *KnowledgeDocumentPolicySnapshot) UnmarshalJSON(raw []byte) error {
	type plain KnowledgeDocumentPolicySnapshot
	var decoded plain
	if err := knowledgeDocumentDecode("PolicySnapshot", raw, &decoded); err != nil {
		return err
	}
	*value = KnowledgeDocumentPolicySnapshot(decoded)
	return nil
}

func (value KnowledgeDocumentPolicySnapshot) MarshalJSON() ([]byte, error) {
	type plain KnowledgeDocumentPolicySnapshot
	return knowledgeDocumentMarshal("PolicySnapshot", plain(value))
}

func (value *KnowledgeDocumentDomainTarget) UnmarshalJSON(raw []byte) error {
	type plain KnowledgeDocumentDomainTarget
	var decoded plain
	if err := knowledgeDocumentDecode("DomainTarget", raw, &decoded); err != nil {
		return err
	}
	*value = KnowledgeDocumentDomainTarget(decoded)
	return nil
}

func (value KnowledgeDocumentDomainTarget) MarshalJSON() ([]byte, error) {
	type plain KnowledgeDocumentDomainTarget
	return knowledgeDocumentMarshal("DomainTarget", plain(value))
}

func (value *KnowledgeDocumentOriginalTarget) UnmarshalJSON(raw []byte) error {
	type plain KnowledgeDocumentOriginalTarget
	var decoded plain
	if err := knowledgeDocumentDecode("OriginalDocumentTarget", raw, &decoded); err != nil {
		return err
	}
	*value = KnowledgeDocumentOriginalTarget(decoded)
	return nil
}

func (value KnowledgeDocumentOriginalTarget) MarshalJSON() ([]byte, error) {
	type plain KnowledgeDocumentOriginalTarget
	return knowledgeDocumentMarshal("OriginalDocumentTarget", plain(value))
}

func (value *KnowledgeDocumentTaskTarget) UnmarshalJSON(raw []byte) error {
	type plain KnowledgeDocumentTaskTarget
	var decoded plain
	if err := knowledgeDocumentDecode("TaskTarget", raw, &decoded); err != nil {
		return err
	}
	*value = KnowledgeDocumentTaskTarget(decoded)
	return nil
}

func (value KnowledgeDocumentTaskTarget) MarshalJSON() ([]byte, error) {
	type plain KnowledgeDocumentTaskTarget
	return knowledgeDocumentMarshal("TaskTarget", plain(value))
}

func (value *KnowledgeDocumentTaskRequest) UnmarshalJSON(raw []byte) error {
	type plain KnowledgeDocumentTaskRequest
	var decoded plain
	if err := knowledgeDocumentDecode("TaskRequest", raw, &decoded); err != nil {
		return err
	}
	*value = KnowledgeDocumentTaskRequest(decoded)
	return nil
}

func (value KnowledgeDocumentTaskRequest) MarshalJSON() ([]byte, error) {
	type plain KnowledgeDocumentTaskRequest
	return knowledgeDocumentMarshal("TaskRequest", plain(value))
}

func (value *KnowledgeDocumentCancelRequest) UnmarshalJSON(raw []byte) error {
	type plain KnowledgeDocumentCancelRequest
	var decoded plain
	if err := knowledgeDocumentDecode("CancelRequest", raw, &decoded); err != nil {
		return err
	}
	*value = KnowledgeDocumentCancelRequest(decoded)
	return nil
}

func (value KnowledgeDocumentCancelRequest) MarshalJSON() ([]byte, error) {
	type plain KnowledgeDocumentCancelRequest
	return knowledgeDocumentMarshal("CancelRequest", plain(value))
}

func (value *KnowledgeDocumentPolicyReceipt) UnmarshalJSON(raw []byte) error {
	type plain KnowledgeDocumentPolicyReceipt
	var decoded plain
	if err := knowledgeDocumentDecode("PolicyReceipt", raw, &decoded); err != nil {
		return err
	}
	*value = KnowledgeDocumentPolicyReceipt(decoded)
	return nil
}

func (value KnowledgeDocumentPolicyReceipt) MarshalJSON() ([]byte, error) {
	type plain KnowledgeDocumentPolicyReceipt
	return knowledgeDocumentMarshal("PolicyReceipt", plain(value))
}

func (value *KnowledgeDocumentTaskProgress) UnmarshalJSON(raw []byte) error {
	type plain KnowledgeDocumentTaskProgress
	var decoded plain
	if err := knowledgeDocumentDecode("TaskProgress", raw, &decoded); err != nil {
		return err
	}
	*value = KnowledgeDocumentTaskProgress(decoded)
	return nil
}

func (value KnowledgeDocumentTaskProgress) MarshalJSON() ([]byte, error) {
	type plain KnowledgeDocumentTaskProgress
	return knowledgeDocumentMarshal("TaskProgress", plain(value))
}

func (value *KnowledgeDocumentTaskDiagnostic) UnmarshalJSON(raw []byte) error {
	type plain KnowledgeDocumentTaskDiagnostic
	var decoded plain
	if err := knowledgeDocumentDecode("TaskDiagnostic", raw, &decoded); err != nil {
		return err
	}
	*value = KnowledgeDocumentTaskDiagnostic(decoded)
	return nil
}

func (value KnowledgeDocumentTaskDiagnostic) MarshalJSON() ([]byte, error) {
	type plain KnowledgeDocumentTaskDiagnostic
	return knowledgeDocumentMarshal("TaskDiagnostic", plain(value))
}

func (value *KnowledgeDocumentTask) UnmarshalJSON(raw []byte) error {
	type plain KnowledgeDocumentTask
	var decoded plain
	if err := knowledgeDocumentDecode("Task", raw, &decoded); err != nil {
		return err
	}
	*value = KnowledgeDocumentTask(decoded)
	return nil
}

func (value KnowledgeDocumentTask) MarshalJSON() ([]byte, error) {
	type plain KnowledgeDocumentTask
	return knowledgeDocumentMarshal("Task", plain(value))
}

func (value *KnowledgeDocumentCancelReceipt) UnmarshalJSON(raw []byte) error {
	type plain KnowledgeDocumentCancelReceipt
	var decoded plain
	if err := knowledgeDocumentDecode("CancelReceipt", raw, &decoded); err != nil {
		return err
	}
	*value = KnowledgeDocumentCancelReceipt(decoded)
	return nil
}

func (value KnowledgeDocumentCancelReceipt) MarshalJSON() ([]byte, error) {
	type plain KnowledgeDocumentCancelReceipt
	return knowledgeDocumentMarshal("CancelReceipt", plain(value))
}

func (value *KnowledgeDocumentTaskRejectionReceipt) UnmarshalJSON(raw []byte) error {
	type plain KnowledgeDocumentTaskRejectionReceipt
	var decoded plain
	if err := knowledgeDocumentDecode("TaskRejectionReceipt", raw, &decoded); err != nil {
		return err
	}
	*value = KnowledgeDocumentTaskRejectionReceipt(decoded)
	return nil
}

func (value KnowledgeDocumentTaskRejectionReceipt) MarshalJSON() ([]byte, error) {
	type plain KnowledgeDocumentTaskRejectionReceipt
	return knowledgeDocumentMarshal("TaskRejectionReceipt", plain(value))
}

func (value *KnowledgeDocumentPolicyRejectionReceipt) UnmarshalJSON(raw []byte) error {
	type plain KnowledgeDocumentPolicyRejectionReceipt
	var decoded plain
	if err := knowledgeDocumentDecode("PolicyRejectionReceipt", raw, &decoded); err != nil {
		return err
	}
	*value = KnowledgeDocumentPolicyRejectionReceipt(decoded)
	return nil
}

func (value KnowledgeDocumentPolicyRejectionReceipt) MarshalJSON() ([]byte, error) {
	type plain KnowledgeDocumentPolicyRejectionReceipt
	return knowledgeDocumentMarshal("PolicyRejectionReceipt", plain(value))
}

func (value *KnowledgeDocumentCancelRejectionReceipt) UnmarshalJSON(raw []byte) error {
	type plain KnowledgeDocumentCancelRejectionReceipt
	var decoded plain
	if err := knowledgeDocumentDecode("CancelRejectionReceipt", raw, &decoded); err != nil {
		return err
	}
	*value = KnowledgeDocumentCancelRejectionReceipt(decoded)
	return nil
}

func (value KnowledgeDocumentCancelRejectionReceipt) MarshalJSON() ([]byte, error) {
	type plain KnowledgeDocumentCancelRejectionReceipt
	return knowledgeDocumentMarshal("CancelRejectionReceipt", plain(value))
}

func (value *KnowledgeDocumentHead) UnmarshalJSON(raw []byte) error {
	type plain KnowledgeDocumentHead
	var decoded plain
	if err := knowledgeDocumentDecode("DocumentHead", raw, &decoded); err != nil {
		return err
	}
	*value = KnowledgeDocumentHead(decoded)
	return nil
}

func (value KnowledgeDocumentHead) MarshalJSON() ([]byte, error) {
	type plain KnowledgeDocumentHead
	return knowledgeDocumentMarshal("DocumentHead", plain(value))
}

func (value *KnowledgeDocumentRepresentation) UnmarshalJSON(raw []byte) error {
	type plain KnowledgeDocumentRepresentation
	var decoded plain
	if err := knowledgeDocumentDecode("Representation", raw, &decoded); err != nil {
		return err
	}
	*value = KnowledgeDocumentRepresentation(decoded)
	return nil
}

func (value KnowledgeDocumentRepresentation) MarshalJSON() ([]byte, error) {
	type plain KnowledgeDocumentRepresentation
	return knowledgeDocumentMarshal("Representation", plain(value))
}

func (value *KnowledgeDocumentNodePage) UnmarshalJSON(raw []byte) error {
	type plain KnowledgeDocumentNodePage
	var decoded plain
	if err := knowledgeDocumentDecode("NodePage", raw, &decoded); err != nil {
		return err
	}
	*value = KnowledgeDocumentNodePage(decoded)
	return nil
}

func (value KnowledgeDocumentNodePage) MarshalJSON() ([]byte, error) {
	type plain KnowledgeDocumentNodePage
	return knowledgeDocumentMarshal("NodePage", plain(value))
}

func (value *KnowledgeDocumentContentWindow) UnmarshalJSON(raw []byte) error {
	type plain KnowledgeDocumentContentWindow
	var decoded plain
	if err := knowledgeDocumentDecode("ContentWindow", raw, &decoded); err != nil {
		return err
	}
	*value = KnowledgeDocumentContentWindow(decoded)
	return nil
}

func (value KnowledgeDocumentContentWindow) MarshalJSON() ([]byte, error) {
	type plain KnowledgeDocumentContentWindow
	return knowledgeDocumentMarshal("ContentWindow", plain(value))
}

func (value *KnowledgeDocumentError) UnmarshalJSON(raw []byte) error {
	type plain KnowledgeDocumentError
	var decoded plain
	if err := knowledgeDocumentDecode("Error", raw, &decoded); err != nil {
		return err
	}
	*value = KnowledgeDocumentError(decoded)
	return nil
}

func (value KnowledgeDocumentError) MarshalJSON() ([]byte, error) {
	type plain KnowledgeDocumentError
	return knowledgeDocumentMarshal("Error", plain(value))
}

func (value *KnowledgeDocumentCapabilitiesResponse) UnmarshalJSON(raw []byte) error {
	type plain KnowledgeDocumentCapabilitiesResponse
	var decoded plain
	if err := knowledgeDocumentDecode("CapabilitiesResponse", raw, &decoded); err != nil {
		return err
	}
	*value = KnowledgeDocumentCapabilitiesResponse(decoded)
	return nil
}

func (value KnowledgeDocumentCapabilitiesResponse) MarshalJSON() ([]byte, error) {
	type plain KnowledgeDocumentCapabilitiesResponse
	return knowledgeDocumentMarshal("CapabilitiesResponse", plain(value))
}

func (value *KnowledgeDocumentPolicyResponse) UnmarshalJSON(raw []byte) error {
	type plain KnowledgeDocumentPolicyResponse
	var decoded plain
	if err := knowledgeDocumentDecode("DocumentPolicyResponse", raw, &decoded); err != nil {
		return err
	}
	*value = KnowledgeDocumentPolicyResponse(decoded)
	return nil
}

func (value KnowledgeDocumentPolicyResponse) MarshalJSON() ([]byte, error) {
	type plain KnowledgeDocumentPolicyResponse
	return knowledgeDocumentMarshal("DocumentPolicyResponse", plain(value))
}

func (value *KnowledgeDocumentPolicyReceiptResponse) UnmarshalJSON(raw []byte) error {
	type plain KnowledgeDocumentPolicyReceiptResponse
	var decoded plain
	if err := knowledgeDocumentDecode("PolicyReceiptResponse", raw, &decoded); err != nil {
		return err
	}
	*value = KnowledgeDocumentPolicyReceiptResponse(decoded)
	return nil
}

func (value KnowledgeDocumentPolicyReceiptResponse) MarshalJSON() ([]byte, error) {
	type plain KnowledgeDocumentPolicyReceiptResponse
	return knowledgeDocumentMarshal("PolicyReceiptResponse", plain(value))
}

func (value *KnowledgeDocumentTaskResponse) UnmarshalJSON(raw []byte) error {
	type plain KnowledgeDocumentTaskResponse
	var decoded plain
	if err := knowledgeDocumentDecode("TaskResponse", raw, &decoded); err != nil {
		return err
	}
	*value = KnowledgeDocumentTaskResponse(decoded)
	return nil
}

func (value KnowledgeDocumentTaskResponse) MarshalJSON() ([]byte, error) {
	type plain KnowledgeDocumentTaskResponse
	return knowledgeDocumentMarshal("TaskResponse", plain(value))
}

func (value *KnowledgeDocumentOperationRecordResponse) UnmarshalJSON(raw []byte) error {
	type plain KnowledgeDocumentOperationRecordResponse
	var decoded plain
	if err := knowledgeDocumentDecode("OperationRecordResponse", raw, &decoded); err != nil {
		return err
	}
	*value = KnowledgeDocumentOperationRecordResponse(decoded)
	return nil
}

func (value KnowledgeDocumentOperationRecordResponse) MarshalJSON() ([]byte, error) {
	type plain KnowledgeDocumentOperationRecordResponse
	return knowledgeDocumentMarshal("OperationRecordResponse", plain(value))
}

func (value *KnowledgeDocumentCancelReceiptResponse) UnmarshalJSON(raw []byte) error {
	type plain KnowledgeDocumentCancelReceiptResponse
	var decoded plain
	if err := knowledgeDocumentDecode("CancelReceiptResponse", raw, &decoded); err != nil {
		return err
	}
	*value = KnowledgeDocumentCancelReceiptResponse(decoded)
	return nil
}

func (value KnowledgeDocumentCancelReceiptResponse) MarshalJSON() ([]byte, error) {
	type plain KnowledgeDocumentCancelReceiptResponse
	return knowledgeDocumentMarshal("CancelReceiptResponse", plain(value))
}

func (value *KnowledgeDocumentHeadResponse) UnmarshalJSON(raw []byte) error {
	type plain KnowledgeDocumentHeadResponse
	var decoded plain
	if err := knowledgeDocumentDecode("DocumentHeadResponse", raw, &decoded); err != nil {
		return err
	}
	*value = KnowledgeDocumentHeadResponse(decoded)
	return nil
}

func (value KnowledgeDocumentHeadResponse) MarshalJSON() ([]byte, error) {
	type plain KnowledgeDocumentHeadResponse
	return knowledgeDocumentMarshal("DocumentHeadResponse", plain(value))
}

func (value *KnowledgeDocumentRepresentationResponse) UnmarshalJSON(raw []byte) error {
	type plain KnowledgeDocumentRepresentationResponse
	var decoded plain
	if err := knowledgeDocumentDecode("RepresentationResponse", raw, &decoded); err != nil {
		return err
	}
	*value = KnowledgeDocumentRepresentationResponse(decoded)
	return nil
}

func (value KnowledgeDocumentRepresentationResponse) MarshalJSON() ([]byte, error) {
	type plain KnowledgeDocumentRepresentationResponse
	return knowledgeDocumentMarshal("RepresentationResponse", plain(value))
}

func (value *KnowledgeDocumentNodePageResponse) UnmarshalJSON(raw []byte) error {
	type plain KnowledgeDocumentNodePageResponse
	var decoded plain
	if err := knowledgeDocumentDecode("NodePageResponse", raw, &decoded); err != nil {
		return err
	}
	*value = KnowledgeDocumentNodePageResponse(decoded)
	return nil
}

func (value KnowledgeDocumentNodePageResponse) MarshalJSON() ([]byte, error) {
	type plain KnowledgeDocumentNodePageResponse
	return knowledgeDocumentMarshal("NodePageResponse", plain(value))
}

func (value *KnowledgeDocumentNodeResponse) UnmarshalJSON(raw []byte) error {
	type plain KnowledgeDocumentNodeResponse
	var decoded plain
	if err := knowledgeDocumentDecode("DocumentNodeResponse", raw, &decoded); err != nil {
		return err
	}
	*value = KnowledgeDocumentNodeResponse(decoded)
	return nil
}

func (value KnowledgeDocumentNodeResponse) MarshalJSON() ([]byte, error) {
	type plain KnowledgeDocumentNodeResponse
	return knowledgeDocumentMarshal("DocumentNodeResponse", plain(value))
}

func (value *KnowledgeDocumentContentWindowResponse) UnmarshalJSON(raw []byte) error {
	type plain KnowledgeDocumentContentWindowResponse
	var decoded plain
	if err := knowledgeDocumentDecode("ContentWindowResponse", raw, &decoded); err != nil {
		return err
	}
	*value = KnowledgeDocumentContentWindowResponse(decoded)
	return nil
}

func (value KnowledgeDocumentContentWindowResponse) MarshalJSON() ([]byte, error) {
	type plain KnowledgeDocumentContentWindowResponse
	return knowledgeDocumentMarshal("ContentWindowResponse", plain(value))
}

func (value *KnowledgeDocumentOperationQuery) UnmarshalJSON(raw []byte) error {
	type plain KnowledgeDocumentOperationQuery
	var decoded plain
	if err := knowledgeDocumentDecode("OperationQuery", raw, &decoded); err != nil {
		return err
	}
	*value = KnowledgeDocumentOperationQuery(decoded)
	return nil
}

func (value KnowledgeDocumentOperationQuery) MarshalJSON() ([]byte, error) {
	type plain KnowledgeDocumentOperationQuery
	return knowledgeDocumentMarshal("OperationQuery", plain(value))
}

func (value *KnowledgeDocumentNodeQuery) UnmarshalJSON(raw []byte) error {
	type plain KnowledgeDocumentNodeQuery
	var decoded plain
	if err := knowledgeDocumentDecode("NodeQuery", raw, &decoded); err != nil {
		return err
	}
	*value = KnowledgeDocumentNodeQuery(decoded)
	return nil
}

func (value KnowledgeDocumentNodeQuery) MarshalJSON() ([]byte, error) {
	type plain KnowledgeDocumentNodeQuery
	return knowledgeDocumentMarshal("NodeQuery", plain(value))
}

func (value *KnowledgeDocumentContentQuery) UnmarshalJSON(raw []byte) error {
	type plain KnowledgeDocumentContentQuery
	var decoded plain
	if err := knowledgeDocumentDecode("ContentQuery", raw, &decoded); err != nil {
		return err
	}
	*value = KnowledgeDocumentContentQuery(decoded)
	return nil
}

func (value KnowledgeDocumentContentQuery) MarshalJSON() ([]byte, error) {
	type plain KnowledgeDocumentContentQuery
	return knowledgeDocumentMarshal("ContentQuery", plain(value))
}

func (value *KnowledgeDocumentOfficeSegment) UnmarshalJSON(raw []byte) error {
	type plain KnowledgeDocumentOfficeSegment
	var decoded plain
	if err := knowledgeDocumentDecode("OfficeSegment", raw, &decoded); err != nil {
		return err
	}
	*value = KnowledgeDocumentOfficeSegment(decoded)
	return nil
}

func (value KnowledgeDocumentOfficeSegment) MarshalJSON() ([]byte, error) {
	type plain KnowledgeDocumentOfficeSegment
	return knowledgeDocumentMarshal("OfficeSegment", plain(value))
}

func (value *KnowledgeDocumentNaturalRef) UnmarshalJSON(raw []byte) error {
	type plain KnowledgeDocumentNaturalRef
	var decoded plain
	if err := knowledgeDocumentDecode("NaturalNodeRef", raw, &decoded); err != nil {
		return err
	}
	*value = KnowledgeDocumentNaturalRef(decoded)
	return nil
}

func (value KnowledgeDocumentNaturalRef) MarshalJSON() ([]byte, error) {
	type plain KnowledgeDocumentNaturalRef
	return knowledgeDocumentMarshal("NaturalNodeRef", plain(value))
}

func (value *KnowledgeDocumentReadingRange) UnmarshalJSON(raw []byte) error {
	type plain KnowledgeDocumentReadingRange
	var decoded plain
	if err := knowledgeDocumentDecode("ReadingRange", raw, &decoded); err != nil {
		return err
	}
	*value = KnowledgeDocumentReadingRange(decoded)
	return nil
}

func (value KnowledgeDocumentReadingRange) MarshalJSON() ([]byte, error) {
	type plain KnowledgeDocumentReadingRange
	return knowledgeDocumentMarshal("ReadingRange", plain(value))
}

func (value *KnowledgeDocumentNodePayload) UnmarshalJSON(raw []byte) error {
	type plain KnowledgeDocumentNodePayload
	var decoded plain
	if err := knowledgeDocumentDecode("NodePayload", raw, &decoded); err != nil {
		return err
	}
	*value = KnowledgeDocumentNodePayload(decoded)
	return nil
}

func (value KnowledgeDocumentNodePayload) MarshalJSON() ([]byte, error) {
	type plain KnowledgeDocumentNodePayload
	return knowledgeDocumentMarshal("NodePayload", plain(value))
}

func (value *KnowledgeDocumentManifestEntry) UnmarshalJSON(raw []byte) error {
	type plain KnowledgeDocumentManifestEntry
	var decoded plain
	if err := knowledgeDocumentDecode("ManifestEntry", raw, &decoded); err != nil {
		return err
	}
	*value = KnowledgeDocumentManifestEntry(decoded)
	return nil
}

func (value KnowledgeDocumentManifestEntry) MarshalJSON() ([]byte, error) {
	type plain KnowledgeDocumentManifestEntry
	return knowledgeDocumentMarshal("ManifestEntry", plain(value))
}

func (value *KnowledgeDocumentStructureManifest) UnmarshalJSON(raw []byte) error {
	type plain KnowledgeDocumentStructureManifest
	var decoded plain
	if err := knowledgeDocumentDecode("StructureManifest", raw, &decoded); err != nil {
		return err
	}
	*value = KnowledgeDocumentStructureManifest(decoded)
	return nil
}

func (value KnowledgeDocumentStructureManifest) MarshalJSON() ([]byte, error) {
	type plain KnowledgeDocumentStructureManifest
	return knowledgeDocumentMarshal("StructureManifest", plain(value))
}

func (value *KnowledgeDocumentNode) UnmarshalJSON(raw []byte) error {
	type plain KnowledgeDocumentNode
	var decoded plain
	if err := knowledgeDocumentDecode("DocumentNode", raw, &decoded); err != nil {
		return err
	}
	*value = KnowledgeDocumentNode(decoded)
	return nil
}

func (value KnowledgeDocumentNode) MarshalJSON() ([]byte, error) {
	type plain KnowledgeDocumentNode
	return knowledgeDocumentMarshal("DocumentNode", plain(value))
}

func (value *KnowledgeDocumentRejectionReceipt) UnmarshalJSON(raw []byte) error {
	type plain KnowledgeDocumentRejectionReceipt
	var decoded plain
	if err := knowledgeDocumentDecode("RejectionReceipt", raw, &decoded); err != nil {
		return err
	}
	*value = KnowledgeDocumentRejectionReceipt(decoded)
	return nil
}

func (value KnowledgeDocumentRejectionReceipt) MarshalJSON() ([]byte, error) {
	type plain KnowledgeDocumentRejectionReceipt
	return knowledgeDocumentMarshal("RejectionReceipt", plain(value))
}
