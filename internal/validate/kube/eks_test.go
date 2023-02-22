package kube

import (
    "github.com/aws/aws-sdk-go-v2/service/ec2/types"
    "github.com/aws/aws-sdk-go-v2/service/eks"
    "github.com/sourcegraph/src-cli/internal/validate"
    "testing"
)

/* func TestValidateEbsCsi(t *testing.T) {
    cases := []struct {
        name   string
        addons func(ListAddonsOutput *eks.ListAddonsOutput)
        result []validate.Result
    }{
        {
            name: "ebs csi drivers installed",
            addons: func(ListAddonsOutput *eks.ListAddonsOutput) {
                ListAddonsOutput.Addons = []string{
                    "aws-ebs-csi-driver",
                }
            },
            result: []validate.Result{
                {
                    Status:  validate.Success,
                    Message: "EKS: ebs-csi driver validated",
                },
            },
        },
        {
            name: "ebs csi drivers not installed",
            addons: func(ListAddonsOutput *eks.ListAddonsOutput) {
                ListAddonsOutput.Addons = []string{}
            },
            result: []validate.Result{
                {
                    Status:  validate.Failure,
                    Message: "EKS: validate ebs-csi driver failed",
                },
            },
        },
    }

    for _, tc := range cases {
        tc := tc
        t.Run(tc.name, func(t *testing.T) {
            addons := testAddonOutput()
            if tc.addons != nil {
                tc.addons(addons)
            }
            result := validateEbsCsiDrivers(&addons.Addons)

            // test should error
            if len(tc.result) > 0 {
                if result == nil {
                    t.Fatal("validate should return result")
                    return
                }
                if result[0].Status != tc.result[0].Status {
                    t.Errorf("result status\nwant: %v\n got: %v", tc.result[0].Status, result[0].Status)
                }
                if result[0].Message != tc.result[0].Message {
                    t.Errorf("result msg\nwant: %s\n got: %s", tc.result[0].Message, result[0].Message)
                }
                return
            }

            // test should not error
            if result != nil {
                t.Fatalf("ValidateService error: %v", result)
            }
        })
    }
} */

func TestValidateVpc(t *testing.T) {
    cases := []struct {
        name   string
        vpc    func(vpc *types.Vpc)
        result []validate.Result
    }{
        {
            name: "valid vpc",
            vpc: func(vpc *types.Vpc) {
                vpc.State = "available"
            },
            result: []validate.Result{
                {
                    Status:  validate.Success,
                    Message: "VPC is validated",
                },
            },
        },
        {
            name: "invalid vpc: pending",
            vpc: func(vpc *types.Vpc) {
                vpc.State = "pending"
            },
            result: []validate.Result{
                {
                    Status:  validate.Failure,
                    Message: "vpc.State stuck in pending state",
                },
            },
        },
    }

    for _, tc := range cases {
        tc := tc
        t.Run(tc.name, func(t *testing.T) {
            vpc := testVPC()
            if tc.vpc != nil {
                tc.vpc(vpc)
            }
            result := validateVpc(vpc)

            // test should error
            if len(tc.result) > 0 {
                if result == nil {
                    t.Fatal("validate should return result")
                    return
                }
                if result[0].Status != tc.result[0].Status {
                    t.Errorf("result status\nwant: %v\n got: %v", tc.result[0].Status, result[0].Status)
                }
                if result[0].Message != tc.result[0].Message {
                    t.Errorf("result msg\nwant: %s\n got: %s", tc.result[0].Message, result[0].Message)
                }
                return
            }

            // test should not error
            if result != nil {
                t.Fatalf("ValidateService error: %v", result)
            }
        })
    }
}

// helper test function to return a valid VPC
func testVPC() *types.Vpc {
    return &types.Vpc{
        State: "available",
    }
}

// helper test function to return a valid list of add-ons
// add-ons are checked for ebs csi drivers
func testAddonOutput() *eks.ListAddonsOutput {
    return &eks.ListAddonsOutput{
        Addons: []string{
            "aws-ebs-csi-driver",
        },
    }
}
