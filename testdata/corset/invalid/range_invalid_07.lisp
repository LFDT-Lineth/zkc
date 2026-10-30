;;error:3:13-14:range constraint for constant column
(defcolumns (x :i16) (c :i32 :constant 7))
(definrange c 4294967296)
