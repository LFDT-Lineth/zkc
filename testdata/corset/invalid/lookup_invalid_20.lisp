;;error:2:14-15:invalid column name
(defcolumns (_ :i1) (X :i16) (Y :i16))
(deflookup test ((_ X)) ((_ Y)))
