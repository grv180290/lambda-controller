// Copyright Amazon.com Inc. or its affiliates. All Rights Reserved.
//
// Licensed under the Apache License, Version 2.0 (the "License"). You may
// not use this file except in compliance with the License. A copy of the
// License is located at
//
//     http://aws.amazon.com/apache2.0/
//
// or in the "license" file accompanying this file. This file is distributed
// on an "AS IS" BASIS, WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either
// express or implied. See the License for the specific language governing
// permissions and limitations under the License.

package alias

import (
	"context"

	ackerr "github.com/aws-controllers-k8s/runtime/pkg/errors"
	ackrtlog "github.com/aws-controllers-k8s/runtime/pkg/runtime/log"
	"github.com/aws/aws-sdk-go/aws"
	svcsdk "github.com/aws/aws-sdk-go/service/lambda"

	svcapitypes "github.com/aws-controllers-k8s/lambda-controller/apis/v1alpha1"
)

func (rm *resourceManager) syncEventInvokeConfig(
	ctx context.Context,
	r *resource,
) (created *resource, err error) {
	rlog := ackrtlog.FromContext(ctx)
	exit := rlog.Trace("rm.syncEventInvokeConfig")
	defer exit(err)

	dspec := r.ko.Spec
	input := &svcsdk.PutFunctionEventInvokeConfigInput{
		FunctionName: aws.String(*dspec.FunctionName),
		Qualifier:    aws.String(*dspec.Name),
	}

	if r.ko.Spec.FunctionEventInvokeConfig != nil {
		if r.ko.Spec.FunctionEventInvokeConfig.DestinationConfig != nil {
			destinations := &svcsdk.DestinationConfig{}
			if r.ko.Spec.FunctionEventInvokeConfig.DestinationConfig.OnFailure != nil {
				destinations.OnFailure = &svcsdk.OnFailure{}
				if r.ko.Spec.FunctionEventInvokeConfig.DestinationConfig.OnFailure.Destination != nil {
					destinations.OnFailure.Destination = aws.String(*r.ko.Spec.FunctionEventInvokeConfig.DestinationConfig.OnFailure.Destination)
				}
			}
			if r.ko.Spec.FunctionEventInvokeConfig.DestinationConfig.OnSuccess != nil {
				destinations.OnSuccess = &svcsdk.OnSuccess{}
				if r.ko.Spec.FunctionEventInvokeConfig.DestinationConfig.OnSuccess.Destination != nil {
					destinations.OnSuccess.Destination = aws.String(*r.ko.Spec.FunctionEventInvokeConfig.DestinationConfig.OnSuccess.Destination)
				}
			}
			input.DestinationConfig = destinations
		}
		if r.ko.Spec.FunctionEventInvokeConfig.MaximumEventAgeInSeconds != nil {
			input.MaximumEventAgeInSeconds = aws.Int64(*r.ko.Spec.FunctionEventInvokeConfig.MaximumEventAgeInSeconds)
		}
		if r.ko.Spec.FunctionEventInvokeConfig.MaximumRetryAttempts != nil {
			input.MaximumRetryAttempts = aws.Int64(*r.ko.Spec.FunctionEventInvokeConfig.MaximumRetryAttempts)
		}
	}
	_, err = rm.sdkapi.PutFunctionEventInvokeConfigWithContext(ctx, input)
	rm.metrics.RecordAPICall("UPDATE", "SyncEventInvokeConfig", err)
	if err != nil {
		return nil, err
	}

	return r, nil
}

// updateProvisionedConcurrency calls `PutProvisionedConcurrencyConfig` to update the fields
// or `DeleteProvisionedConcurrencyConfig` if users removes the fields
func (rm *resourceManager) updateProvisionedConcurrency(
	ctx context.Context,
	desired *resource,
) error {
	var err error
	rlog := ackrtlog.FromContext(ctx)
	exit := rlog.Trace("rm.updateProvisionedConcurrency")
	defer exit(err)

	// Check if the user deleted the 'ProvisionedConcurrency' configuration
	// If yes, delete ProvisionedConcurrencyConfig
	if desired.ko.Spec.ProvisionedConcurrencyConfig == nil || desired.ko.Spec.ProvisionedConcurrencyConfig.ProvisionedConcurrentExecutions == nil {
		input_delete := &svcsdk.DeleteProvisionedConcurrencyConfigInput{
			FunctionName: aws.String(*desired.ko.Spec.FunctionName),
			Qualifier:    aws.String(*desired.ko.Spec.Name),
		}
		_, err = rm.sdkapi.DeleteProvisionedConcurrencyConfigWithContext(ctx, input_delete)
		rm.metrics.RecordAPICall("DELETE", "DeleteProvisionedConcurrency", err)
		if err != nil {
			return err
		}
		return nil
	}

	dspec := desired.ko.Spec
	input := &svcsdk.PutProvisionedConcurrencyConfigInput{
		FunctionName:                    aws.String(*dspec.FunctionName),
		Qualifier:                       aws.String(*dspec.Name),
		ProvisionedConcurrentExecutions: aws.Int64(*dspec.ProvisionedConcurrencyConfig.ProvisionedConcurrentExecutions),
	}

	_, err = rm.sdkapi.PutProvisionedConcurrencyConfigWithContext(ctx, input)
	rm.metrics.RecordAPICall("UPDATE", "UpdateProvisionedConcurrency", err)
	if err != nil {
		return err
	}
	return nil
}

// setProvisionedConcurrencyConfig sets the Provisioned Concurrency
// for the Function's Alias
func (rm *resourceManager) setProvisionedConcurrencyConfig(
	ctx context.Context,
	ko *svcapitypes.Alias,
) (err error) {
	rlog := ackrtlog.FromContext(ctx)
	exit := rlog.Trace("rm.setProvisionedConcurrencyConfig")
	defer exit(err)

	var getProvisionedConcurrencyConfigOutput *svcsdk.GetProvisionedConcurrencyConfigOutput
	getProvisionedConcurrencyConfigOutput, err = rm.sdkapi.GetProvisionedConcurrencyConfigWithContext(
		ctx,
		&svcsdk.GetProvisionedConcurrencyConfigInput{
			FunctionName: ko.Spec.FunctionName,
			Qualifier:    ko.Spec.Name,
		},
	)
	rm.metrics.RecordAPICall("GET", "GetProvisionedConcurrencyConfig", err)

	if err != nil {
		if awserr, ok := ackerr.AWSError(err); ok && (awserr.Code() == "ProvisionedConcurrencyConfigNotFoundException" || awserr.Code() == "ResourceNotFoundException") {
			ko.Spec.ProvisionedConcurrencyConfig = nil
		} else {
			return err
		}
	} else {
		// creating ProvisionedConcurrency object to store the values returned from `Get` call
		cloudProvisionedConcurrency := &svcapitypes.PutProvisionedConcurrencyConfigInput{}
		cloudProvisionedConcurrency.ProvisionedConcurrentExecutions = getProvisionedConcurrencyConfigOutput.RequestedProvisionedConcurrentExecutions
		ko.Spec.ProvisionedConcurrencyConfig = cloudProvisionedConcurrency
	}

	return nil
}

// getFunctionEventInvokeConfig will describe the fields that are
// custom to the Alias resource
func (rm *resourceManager) getFunctionEventInvokeConfig(
	ctx context.Context,
	ko *svcapitypes.Alias,
) (err error) {
	var getFunctionEventInvokeConfigOutput *svcsdk.GetFunctionEventInvokeConfigOutput
	getFunctionEventInvokeConfigOutput, err = rm.sdkapi.GetFunctionEventInvokeConfigWithContext(
		ctx,
		&svcsdk.GetFunctionEventInvokeConfigInput{
			FunctionName: ko.Spec.FunctionName,
			Qualifier:    ko.Spec.Name,
		},
	)

	rm.metrics.RecordAPICall("GET", "GetFunctionEventInvokeConfig", err)

	if err != nil {
		if awserr, ok := ackerr.AWSError(err); ok && (awserr.Code() == "EventInvokeConfigNotFoundException" || awserr.Code() == "ResourceNotFoundException") {
			ko.Spec.FunctionEventInvokeConfig = nil
		} else {
			return err
		}
	} else {
		if getFunctionEventInvokeConfigOutput.DestinationConfig != nil {
			if getFunctionEventInvokeConfigOutput.DestinationConfig.OnFailure != nil {
				if getFunctionEventInvokeConfigOutput.DestinationConfig.OnFailure.Destination != nil {
					ko.Spec.FunctionEventInvokeConfig.DestinationConfig.OnFailure.Destination = getFunctionEventInvokeConfigOutput.DestinationConfig.OnFailure.Destination
				}
			}
			if getFunctionEventInvokeConfigOutput.DestinationConfig.OnSuccess != nil {
				if getFunctionEventInvokeConfigOutput.DestinationConfig.OnSuccess.Destination != nil {
					ko.Spec.FunctionEventInvokeConfig.DestinationConfig.OnSuccess.Destination = getFunctionEventInvokeConfigOutput.DestinationConfig.OnSuccess.Destination
				}
			}
		} else {
			ko.Spec.FunctionEventInvokeConfig.DestinationConfig = nil
		}
		if getFunctionEventInvokeConfigOutput.MaximumEventAgeInSeconds != nil {
			ko.Spec.FunctionEventInvokeConfig.MaximumEventAgeInSeconds = getFunctionEventInvokeConfigOutput.MaximumEventAgeInSeconds
		} else {
			ko.Spec.FunctionEventInvokeConfig.MaximumEventAgeInSeconds = nil
		}
		if getFunctionEventInvokeConfigOutput.DestinationConfig != nil {
			ko.Spec.FunctionEventInvokeConfig.MaximumRetryAttempts = getFunctionEventInvokeConfigOutput.MaximumRetryAttempts
		} else {
			ko.Spec.FunctionEventInvokeConfig.MaximumRetryAttempts = nil
		}
	}

	return nil
}

// setResourceAdditionalFields will describe the fields that are not return by the
// getFunctionConfiguration API call
func (rm *resourceManager) setResourceAdditionalFields(
	ctx context.Context,
	ko *svcapitypes.Alias,
) (err error) {
	rlog := ackrtlog.FromContext(ctx)
	exit := rlog.Trace("rm.setResourceAdditionalFields")
	defer exit(err)

	// To set Asynchronous Invocations for the function's alias
	eic_err := rm.getFunctionEventInvokeConfig(ctx, ko)
	if eic_err != nil {
		return eic_err
	}

	// To set Provisioned Concurrency for the function's alias
	err = rm.setProvisionedConcurrencyConfig(ctx, ko)
	if err != nil {
		return err
	}

	return nil
}
