;;error:3:23-24:expected u16, found u17
(defcolumns (X :i17) (Y :i16))
(defpermutation p ((_ X)) ((_ Y)))
