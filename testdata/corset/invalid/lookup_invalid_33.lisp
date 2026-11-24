;;error:3:21-22:expected u16, found u17
(defcolumns (X :i17) (Y :i16))
(deflookup test ((_ X)) ((_ Y)))
