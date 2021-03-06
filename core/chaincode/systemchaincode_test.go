/*
Copyright IBM Corp. 2016 All Rights Reserved.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

		 http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package chaincode

import (
	"net"
	"os"
	"testing"
	"time"

	"github.com/hyperledger/fabric/common/util"
	"github.com/hyperledger/fabric/core/peer"
	"github.com/hyperledger/fabric/core/system_chaincode/samplesyscc"
	pb "github.com/hyperledger/fabric/protos/peer"
	"github.com/spf13/viper"
	"golang.org/x/net/context"
	"google.golang.org/grpc"
)

type oldSysCCInfo struct {
	origSystemCC       []*SystemChaincode
	origSysCCWhitelist map[string]string
}

func (osyscc *oldSysCCInfo) reset() {
	systemChaincodes = osyscc.origSystemCC
	viper.Set("chaincode.system", osyscc.origSysCCWhitelist)
}

func initSysCCTests() (*oldSysCCInfo, net.Listener, error) {
	var opts []grpc.ServerOption
	grpcServer := grpc.NewServer(opts...)
	viper.Set("peer.fileSystemPath", "/tmp/hyperledger/test/tmpdb")
	defer os.RemoveAll("/tmp/hyperledger/test/tmpdb")

	peer.MockInitialize()

	//use a different address than what we usually use for "peer"
	//we override the peerAddress set in chaincode_support.go
	// FIXME: Use peer.GetLocalAddress()
	peerAddress := "0.0.0.0:21726"
	lis, err := net.Listen("tcp", peerAddress)
	if err != nil {
		return nil, nil, err
	}

	getPeerEndpoint := func() (*pb.PeerEndpoint, error) {
		return &pb.PeerEndpoint{ID: &pb.PeerID{Name: "testpeer"}, Address: peerAddress}, nil
	}

	ccStartupTimeout := time.Duration(5000) * time.Millisecond
	pb.RegisterChaincodeSupportServer(grpcServer, NewChaincodeSupport(getPeerEndpoint, false, ccStartupTimeout))

	go grpcServer.Serve(lis)

	sysccinfo := &oldSysCCInfo{systemChaincodes, viper.GetStringMapString("chaincode.system")}

	//set systemChaincodes to sample
	systemChaincodes = []*SystemChaincode{
		{
			Enabled:   true,
			Name:      "sample_syscc",
			Path:      "github.com/hyperledger/fabric/core/system_chaincode/samplesyscc",
			InitArgs:  [][]byte{},
			Chaincode: &samplesyscc.SampleSysCC{},
		},
	}

	// System chaincode has to be enabled
	viper.Set("chaincode.system", map[string]string{"sample_syscc": "true"})

	RegisterSysCCs()

	/////^^^ system initialization completed ^^^
	return sysccinfo, lis, nil
}

func deploySampleSysCC(t *testing.T, ctxt context.Context, chainID string) error {
	DeploySysCCs(chainID)

	url := "github.com/hyperledger/fabric/core/system_chaincode/sample_syscc"

	cdsforStop := &pb.ChaincodeDeploymentSpec{ExecEnv: 1, ChaincodeSpec: &pb.ChaincodeSpec{Type: 1, ChaincodeID: &pb.ChaincodeID{Name: "sample_syscc", Path: url}, CtorMsg: &pb.ChaincodeInput{Args: [][]byte{[]byte("")}}}}

	f := "putval"
	args := util.ToChaincodeArgs(f, "greeting", "hey there")

	spec := &pb.ChaincodeSpec{Type: 1, ChaincodeID: &pb.ChaincodeID{Name: "sample_syscc", Path: url}, CtorMsg: &pb.ChaincodeInput{Args: args}}

	sysCCVers := util.GetSysCCVersion()

	_, _, _, err := invokeWithVersion(ctxt, chainID, sysCCVers, spec)

	cccid := NewCCContext(chainID, "sample_syscc", sysCCVers, "", true, nil)
	if err != nil {
		theChaincodeSupport.Stop(ctxt, cccid, cdsforStop)
		t.Logf("Error invoking sample_syscc: %s", err)
		return err
	}

	f = "getval"
	args = util.ToChaincodeArgs(f, "greeting")
	spec = &pb.ChaincodeSpec{Type: 1, ChaincodeID: &pb.ChaincodeID{Name: "sample_syscc", Path: url}, CtorMsg: &pb.ChaincodeInput{Args: args}}
	_, _, _, err = invokeWithVersion(ctxt, chainID, sysCCVers, spec)
	if err != nil {
		theChaincodeSupport.Stop(ctxt, cccid, cdsforStop)
		t.Logf("Error invoking sample_syscc: %s", err)
		return err
	}

	theChaincodeSupport.Stop(ctxt, cccid, cdsforStop)

	return nil
}

// Test deploy of a transaction.
func TestExecuteDeploySysChaincode(t *testing.T) {
	sysccinfo, lis, err := initSysCCTests()
	if err != nil {
		t.Fail()
		return
	}

	defer func() {
		sysccinfo.reset()
	}()

	chainID := util.GetTestChainID()

	if err = peer.MockCreateChain(chainID); err != nil {
		closeListenerAndSleep(lis)
		return
	}

	var ctxt = context.Background()

	err = deploySampleSysCC(t, ctxt, chainID)
	if err != nil {
		closeListenerAndSleep(lis)
		t.Fail()
		return
	}

	closeListenerAndSleep(lis)
}

// Test multichains
func TestMultichains(t *testing.T) {
	sysccinfo, lis, err := initSysCCTests()
	if err != nil {
		t.Fail()
		return
	}

	defer func() {
		sysccinfo.reset()
	}()

	chainID := "chain1"

	if err = peer.MockCreateChain(chainID); err != nil {
		closeListenerAndSleep(lis)
		return
	}

	var ctxt = context.Background()

	err = deploySampleSysCC(t, ctxt, chainID)
	if err != nil {
		closeListenerAndSleep(lis)
		t.Fail()
		return
	}

	chainID = "chain2"

	if err = peer.MockCreateChain(chainID); err != nil {
		closeListenerAndSleep(lis)
		return
	}

	err = deploySampleSysCC(t, ctxt, chainID)
	if err != nil {
		closeListenerAndSleep(lis)
		t.Fail()
		return
	}

	closeListenerAndSleep(lis)
}
