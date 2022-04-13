package main

import (
	"fmt"
	"io/ioutil"
	"log"
	"strings"

	"github.com/mingmxren/protokit"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/descriptorpb"
	"google.golang.org/protobuf/types/pluginpb"
	"gopkg.in/yaml.v2"
)

const (
	SyntaxProto3 = "proto3"
	SyntaxProto2 = "proto2"
)

type PluginOptions struct {
	PackageReplace map[string]string `yaml:"PackageReplace"`
	ImportReplace  map[string]string `yaml:"ImportReplace"`
	DeleteValidate bool              `yaml:"DeleteValidate"`
	TargetSyntax   string            `yaml:"TargetSyntax"`
}

func (po *PluginOptions) ParseOptions(parameter string) {
	yamlFile, err := ioutil.ReadFile(parameter)
	if err != nil {
		log.Fatal(err)
	}
	err = yaml.Unmarshal(yamlFile, po)
	if err != nil {
		log.Fatal(err)
	}
}

func NewPluginOptions() *PluginOptions {
	po := new(PluginOptions)
	return po
}

type Plugin struct {
	Opts *PluginOptions
}

func NewPlugin() (pi *Plugin) {
	pi = new(Plugin)
	pi.Opts = NewPluginOptions()

	return pi
}

func (pi *Plugin) ReplacePath(name string) string {
	if len(name) == 0 {
		return ""
	}
	for source, target := range pi.Opts.ImportReplace {
		if strings.HasPrefix(name, source) {
			return strings.Replace(name, source, target, 1)
		}
	}
	return name
}

func (pi *Plugin) ReplacePackage(name string) string {
	if len(name) == 0 {
		return ""
	}
	pn := name
	if name[0] == '.' {
		pn = name[1:]
	}
	for source, target := range pi.Opts.PackageReplace {
		if strings.HasPrefix(pn, source) {
			return strings.Replace(name, source, target, 1)
		}
	}
	return name
}

func (pi *Plugin) GetStringLabel(label descriptorpb.FieldDescriptorProto_Label) string {
	if label == descriptorpb.FieldDescriptorProto_LABEL_REPEATED {
		return "repeated"
	}

	if pi.Opts.TargetSyntax == SyntaxProto3 {
		if label == descriptorpb.FieldDescriptorProto_LABEL_OPTIONAL {
			return ""
		}
	} else if pi.Opts.TargetSyntax == SyntaxProto2 {
		if label == descriptorpb.FieldDescriptorProto_LABEL_OPTIONAL {
			return "optional"
		} else if label == descriptorpb.FieldDescriptorProto_LABEL_REQUIRED {
			return "required"
		}
	}
	log.Fatal(fmt.Sprintf("error label: %s TargetSyntax:%s", label, pi.Opts.TargetSyntax))
	return ""
}

func (pi *Plugin) GenMessageDefine(msg *protokit.PKDescriptor) string {
	sb := new(strings.Builder)
	sb.WriteString(fmt.Sprintf("message %s {\n", msg.GetName()))
	for _, subMsg := range msg.GetMessages() {
		sb.WriteString(Indent(WithComments(pi.GenMessageDefine(subMsg), subMsg.Comments, 4), 4))
	}
	for _, subEnum := range msg.GetEnums() {
		sb.WriteString(Indent(WithComments(pi.GenEnumDefine(subEnum), subEnum.Comments, 4), 4))
	}
	for _, field := range msg.GetMessageFields() {
		sb.WriteString(WithComments(
			fmt.Sprintf("    %s %s %s = %d;\n",
				pi.GetStringLabel(field.GetLabel()), pi.ReplacePackage(GetStringType(field)),
				field.GetName(), field.GetNumber(),
			), field.Comments, 4))
	}
	sb.WriteString("}\n")
	return sb.String()
}
func (pi *Plugin) GenEnumDefine(enum *protokit.PKEnumDescriptor) string {
	sb := new(strings.Builder)
	sb.WriteString(fmt.Sprintf("enum %s {\n", enum.GetName()))
	for _, val := range enum.GetValues() {
		sb.WriteString(WithComments(fmt.Sprintf("    %s = %d;\n", val.GetName(), val.GetNumber()), val.Comments, 4))
	}
	sb.WriteString("}\n")
	return sb.String()
}

func (pi *Plugin) GenMethodDefine(method *protokit.PKMethodDescriptor) string {
	sb := new(strings.Builder)
	sb.WriteString(fmt.Sprintf("rpc %s(%s) returns (%s) {\n", method.GetName(),
		pi.ReplacePackage(method.GetInputType()), pi.ReplacePackage(method.GetOutputType())))
	for optName, opt := range method.OptionExtensions {
		sb.WriteString(fmt.Sprintf("    option (%s) = \"%s\";\n", optName, opt))
	}
	sb.WriteString("}\n")

	return sb.String()
}

func (pi *Plugin) GenServiceDefine(service *protokit.PKServiceDescriptor) string {
	sb := new(strings.Builder)
	sb.WriteString(fmt.Sprintf("service %s {\n", service.GetName()))
	for _, method := range service.GetMethods() {
		sb.WriteString(WithComments(Indent(pi.GenMethodDefine(method), 4), method.Comments, 4))
	}

	sb.WriteString("}\n")

	return sb.String()

}

func (pi *Plugin) DealFile(pf *protokit.PKFileDescriptor) (*pluginpb.CodeGeneratorResponse_File, error) {
	if pf.GetSyntax() == "proto2" {
		return nil, nil
	}

	rf := new(pluginpb.CodeGeneratorResponse_File)

	rf.Name = new(string)
	*rf.Name = pi.ReplacePath(*pf.Name)

	pb := new(strings.Builder)

	pb.WriteString(WithComments(fmt.Sprintf("syntax = \"%s\";\n", pi.Opts.TargetSyntax), pf.SyntaxComments, 0))
	pb.WriteString(WithComments(fmt.Sprintf("package %s;\n", pi.ReplacePackage(pf.GetPackage())), pf.PackageComments,
		0))
	for _, dep := range pf.Dependency {
		if pi.Opts.DeleteValidate {
			if dep == "validate/validate.proto" {
				continue
			}
		}
		pb.WriteString(fmt.Sprintf("import \"%s\";\n", pi.ReplacePath(dep)))
	}
	if pf.GetOptions().GetCcGenericServices() {
		pb.WriteString("option cc_generic_services = true;\n")
	}
	for _, enum := range pf.GetEnums() {
		pb.WriteString(WithComments(pi.GenEnumDefine(enum), enum.Comments, 0))
	}
	for _, msg := range pf.GetMessages() {
		pb.WriteString(WithComments(pi.GenMessageDefine(msg), msg.Comments, 0))

	}

	for _, service := range pf.GetServices() {
		pb.WriteString(WithComments(pi.GenServiceDefine(service), service.Comments, 0))
	}

	rf.Content = new(string)
	*rf.Content = pb.String()

	return rf, nil
}

func (pi *Plugin) Generate(req *pluginpb.CodeGeneratorRequest) (*pluginpb.CodeGeneratorResponse, error) {
	rsp := &pluginpb.CodeGeneratorResponse{
		SupportedFeatures: proto.Uint64(uint64(pluginpb.CodeGeneratorResponse_FEATURE_PROTO3_OPTIONAL)),
	}
	// only one parameter: a yaml file name
	pi.Opts.ParseOptions(req.GetParameter())

	allFiles, err := protokit.ParseCodeGenRequestAllFiles(req)
	if err != nil {
		return nil, err
	}
	for _, pf := range allFiles {
		if !pf.IsFileToGenerate {
			continue
		}
		rspFile, err := pi.DealFile(pf)
		if err != nil {
			return nil, err
		}
		if rspFile != nil {
			rsp.File = append(rsp.File, rspFile)
		}
	}

	return rsp, nil
}
